package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/under-the-bridge-hq/slack_crawler/internal/config"
	"github.com/under-the-bridge-hq/slack_crawler/internal/crawler"
	"github.com/under-the-bridge-hq/slack_crawler/internal/model"
	"github.com/under-the-bridge-hq/slack_crawler/internal/store"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "slack-crawler",
		Short: "SlackチャンネルのメッセージをクロールしてSQLiteに保存するCLIツール",
	}
	root.AddCommand(crawlCmd())
	root.AddCommand(channelsCmd())
	root.AddCommand(statsCmd())
	root.AddCommand(queryCmd())
	root.AddCommand(crawlSearchCmd())
	return root
}

func crawlCmd() *cobra.Command {
	var (
		channelFlag string
		sinceFlag   string
		untilFlag   string
	)

	cmd := &cobra.Command{
		Use:   "crawl",
		Short: "指定チャンネルのメッセージをクロールしてSQLiteに保存",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			// --channel フラグで上書き
			if channelFlag != "" {
				cfg.Channels = []config.ChannelEntry{{ID: channelFlag}}
			}

			if err := cfg.ValidateForCrawl(); err != nil {
				return err
			}

			logger := newLogger(cfg.LogLevel)

			// dataディレクトリの自動作成
			if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
				return fmt.Errorf("dataディレクトリ作成: %w", err)
			}

			s, err := store.New(cfg.DBPath)
			if err != nil {
				return err
			}
			defer s.Close()

			client := crawler.NewSlackClient(cfg.SlackToken())
			cr := crawler.New(client, s, logger)
			cr.SetProgress(func(format string, args ...any) {
				fmt.Fprintf(os.Stderr, "\r\033[K"+format, args...)
			})

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			for _, chID := range cfg.ChannelIDs() {
				logger.Info("チャンネル情報を取得", "channel_id", chID)
				ch, err := cr.FetchChannel(ctx, chID)
				if err != nil {
					return fmt.Errorf("チャンネル %s の情報取得失敗: %w", chID, err)
				}
				logger.Info("チャンネル情報取得完了", "name", ch.Name)

				// クロールログ開始
				logID, err := s.StartCrawlLog(ctx, chID)
				if err != nil {
					return fmt.Errorf("crawl_log開始失敗: %w", err)
				}

				// スレッド差分用: クロール前のreply_countを保存
				storedCounts, err := s.GetStoredReplyCount(ctx, chID)
				if err != nil {
					s.FailCrawlLog(ctx, logID, err.Error())
					return fmt.Errorf("保存済みreply_count取得失敗: %w", err)
				}

				// 期間指定: --sinceが指定されていればそちらを優先、なければ差分クロール
				oldest := ""
				if sinceFlag != "" {
					ts, err := dateToSlackTS(sinceFlag)
					if err != nil {
						s.FailCrawlLog(ctx, logID, err.Error())
						return err
					}
					oldest = ts
					logger.Info("期間指定クロール", "since", sinceFlag)
				} else {
					oldest, _ = s.GetLatestTS(ctx, chID)
					if oldest != "" {
						logger.Info("差分クロール", "oldest", oldest)
					}
				}

				latest := ""
				if untilFlag != "" {
					ts, err := dateToSlackTS(untilFlag)
					if err != nil {
						s.FailCrawlLog(ctx, logID, err.Error())
						return err
					}
					latest = ts
					logger.Info("期間指定クロール", "until", untilFlag)
				}

				total, err := cr.CrawlMessages(ctx, chID, oldest, latest)
				if err != nil {
					s.FailCrawlLog(ctx, logID, err.Error())
					return fmt.Errorf("チャンネル %s のクロール失敗: %w", chID, err)
				}
				logger.Info("メッセージクロール完了", "channel", ch.Name, "messages", total)

				// スレッド返信を取得（差分: reply_countが変わったスレッドのみ）
				threadTotal := 0
				parents, err := s.GetThreadParents(ctx, chID)
				if err != nil {
					s.FailCrawlLog(ctx, logID, err.Error())
					return fmt.Errorf("スレッド親取得失敗: %w", err)
				}
				// reply_countが変わったスレッドだけフィルタ
				var updatedParents []*model.Message
				for _, p := range parents {
					prev, exists := storedCounts[p.TS]
					if !exists || p.ReplyCount != prev {
						updatedParents = append(updatedParents, p)
					}
				}
				if len(updatedParents) > 0 {
					logger.Info("スレッド返信を取得中", "updated_threads", len(updatedParents), "total_threads", len(parents))
					threadTotal, err = cr.CrawlThreadReplies(ctx, chID, updatedParents)
					if err != nil {
						s.FailCrawlLog(ctx, logID, err.Error())
						return fmt.Errorf("スレッド返信クロール失敗: %w", err)
					}
					logger.Info("スレッド返信クロール完了", "replies", threadTotal)
				} else {
					logger.Info("スレッド返信の更新なし")
				}

				// クロールログ完了
				if err := s.CompleteCrawlLog(ctx, logID, total, threadTotal); err != nil {
					return fmt.Errorf("crawl_log完了記録失敗: %w", err)
				}
			}

			fmt.Fprint(os.Stderr, "\r\033[K") // 進捗行クリア

			// 未登録ユーザーの情報を取得
			userIDs, err := s.GetDistinctUserIDs(ctx)
			if err != nil {
				return fmt.Errorf("未登録ユーザーID取得失敗: %w", err)
			}
			if len(userIDs) > 0 {
				logger.Info("ユーザー情報を取得中", "users", len(userIDs))
				uCount, err := cr.FetchUsers(ctx, userIDs)
				if err != nil {
					return fmt.Errorf("ユーザー情報取得失敗: %w", err)
				}
				logger.Info("ユーザー情報取得完了", "fetched", uCount)
			}

			fmt.Println("完了")
			return nil
		},
	}
	cmd.Flags().StringVar(&channelFlag, "channel", "", "クロール対象チャンネルID（環境変数より優先）")
	cmd.Flags().StringVar(&sinceFlag, "since", "", "取得開始日 (例: 2026-01-01)")
	cmd.Flags().StringVar(&untilFlag, "until", "", "取得終了日 (例: 2026-03-31)")
	return cmd
}

func channelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "channels",
		Short: "保存済みチャンネル一覧を表示",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			s, err := store.New(cfg.DBPath)
			if err != nil {
				return err
			}
			defer s.Close()

			channels, err := s.ListChannels(context.Background())
			if err != nil {
				return err
			}

			if len(channels) == 0 {
				fmt.Println("保存済みチャンネルはありません。先に crawl を実行してください。")
				return nil
			}

			for _, ch := range channels {
				private := ""
				if ch.IsPrivate {
					private = " (private)"
				}
				fmt.Printf("%-12s %s%s\n", ch.ID, ch.Name, private)
			}
			return nil
		},
	}
}

func statsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "保存済みデータの統計を表示",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			s, err := store.New(cfg.DBPath)
			if err != nil {
				return err
			}
			defer s.Close()

			ctx := context.Background()
			channels, err := s.ListChannels(ctx)
			if err != nil {
				return err
			}

			if len(channels) == 0 {
				fmt.Println("データなし")
				return nil
			}

			for _, ch := range channels {
				count, _ := s.CountMessages(ctx, ch.ID)
				fmt.Printf("%-12s %-20s %d messages\n", ch.ID, ch.Name, count)
			}
			return nil
		},
	}
}

func crawlSearchCmd() *cobra.Command {
	var (
		userFlag  string
		sinceFlag string
		untilFlag string
		queryFlag string
	)

	cmd := &cobra.Command{
		Use:   "crawl-search",
		Short: "Slack検索APIでメッセージを収集してSQLiteに保存",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}

			// 検索クエリ構築
			q := queryFlag
			if q == "" {
				if userFlag == "" {
					return fmt.Errorf("--user または --query を指定してください")
				}
				q = fmt.Sprintf("from:<@%s>", userFlag)
				if sinceFlag != "" {
					q += fmt.Sprintf(" after:%s", sinceFlag)
				}
				if untilFlag != "" {
					q += fmt.Sprintf(" before:%s", untilFlag)
				}
			}

			logger := newLogger(cfg.LogLevel)
			logger.Info("検索クエリ", "query", q)

			if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
				return fmt.Errorf("dataディレクトリ作成: %w", err)
			}

			s, err := store.New(cfg.DBPath)
			if err != nil {
				return err
			}
			defer s.Close()

			client := crawler.NewSlackClient(cfg.SlackToken())
			cr := crawler.New(client, s, logger)
			cr.SetProgress(func(format string, args ...any) {
				fmt.Fprintf(os.Stderr, "\r\033[K"+format, args...)
			})

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			total, err := cr.SearchCrawl(ctx, q)
			if err != nil {
				return fmt.Errorf("検索クロール失敗: %w", err)
			}

			fmt.Fprintf(os.Stderr, "\r\033[K")
			fmt.Printf("完了: %d件のメッセージを保存\n", total)
			return nil
		},
	}

	cmd.Flags().StringVar(&userFlag, "user", "", "対象ユーザーID (例: U07KF1QFLRY)")
	cmd.Flags().StringVar(&sinceFlag, "since", "", "開始日 (例: 2026-01-01)")
	cmd.Flags().StringVar(&untilFlag, "until", "", "終了日 (例: 2026-03-31)")
	cmd.Flags().StringVar(&queryFlag, "query", "", "Slack検索クエリを直接指定（--userより優先）")
	return cmd
}

func queryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "query [SQL]",
		Short: "SQLiteに直接クエリを実行",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			s, err := store.New(cfg.DBPath)
			if err != nil {
				return err
			}
			defer s.Close()

			columns, rows, err := s.RawQuery(context.Background(), args[0])
			if err != nil {
				return fmt.Errorf("クエリ実行失敗: %w", err)
			}

			if len(rows) == 0 {
				fmt.Println("結果なし")
				return nil
			}

			// カラム幅を計算
			widths := make([]int, len(columns))
			for i, col := range columns {
				widths[i] = len(col)
			}
			for _, row := range rows {
				for i, val := range row {
					if len(val) > widths[i] {
						widths[i] = len(val)
					}
					// 幅を最大50文字に制限
					if widths[i] > 50 {
						widths[i] = 50
					}
				}
			}

			// ヘッダー出力
			for i, col := range columns {
				fmt.Printf("%-*s  ", widths[i], col)
			}
			fmt.Println()
			for i := range columns {
				for j := 0; j < widths[i]; j++ {
					fmt.Print("-")
				}
				fmt.Print("  ")
			}
			fmt.Println()

			// 行出力
			for _, row := range rows {
				for i, val := range row {
					if len(val) > 50 {
						val = val[:47] + "..."
					}
					fmt.Printf("%-*s  ", widths[i], val)
				}
				fmt.Println()
			}

			fmt.Printf("\n(%d rows)\n", len(rows))
			return nil
		},
	}
}

func dateToSlackTS(dateStr string) (string, error) {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return "", fmt.Errorf("日付形式が不正です（YYYY-MM-DD）: %w", err)
	}
	return fmt.Sprintf("%d.000000", t.Unix()), nil
}

func newLogger(level string) *slog.Logger {
	var lv slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lv = slog.LevelDebug
	case "warn":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lv}))
}

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/under-the-bridge-hq/slack_crawler/internal/config"
	"github.com/under-the-bridge-hq/slack_crawler/internal/crawler"
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
	return root
}

func crawlCmd() *cobra.Command {
	var channelFlag string

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
				cfg.ChannelIDs = []string{channelFlag}
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

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			for _, chID := range cfg.ChannelIDs {
				logger.Info("チャンネル情報を取得", "channel_id", chID)
				ch, err := cr.FetchChannel(ctx, chID)
				if err != nil {
					return fmt.Errorf("チャンネル %s の情報取得失敗: %w", chID, err)
				}
				logger.Info("チャンネル情報取得完了", "name", ch.Name)

				// 差分クロール: 前回の最新TSを取得
				oldest, _ := s.GetLatestTS(ctx, chID)
				if oldest != "" {
					logger.Info("差分クロール", "oldest", oldest)
				}

				total, err := cr.CrawlMessages(ctx, chID, oldest)
				if err != nil {
					return fmt.Errorf("チャンネル %s のクロール失敗: %w", chID, err)
				}
				logger.Info("クロール完了", "channel", ch.Name, "messages", total)
			}

			fmt.Println("完了")
			return nil
		},
	}
	cmd.Flags().StringVar(&channelFlag, "channel", "", "クロール対象チャンネルID（環境変数より優先）")
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

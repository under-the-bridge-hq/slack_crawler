package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/slack-go/slack"

	"github.com/under-the-bridge-hq/slack_crawler/internal/model"
	"github.com/under-the-bridge-hq/slack_crawler/internal/store"
)

// ProgressFunc は進捗表示用コールバック。
type ProgressFunc func(format string, args ...any)

// Crawler はSlack APIからメッセージを取得しStoreに保存する。
type Crawler struct {
	client   SlackClient
	store    *store.Store
	logger   *slog.Logger
	progress ProgressFunc
}

func New(client SlackClient, store *store.Store, logger *slog.Logger) *Crawler {
	return &Crawler{client: client, store: store, logger: logger, progress: func(string, ...any) {}}
}

// SetProgress は進捗表示コールバックを設定する。
func (c *Crawler) SetProgress(fn ProgressFunc) {
	c.progress = fn
}

// FetchChannel はチャンネル情報を取得しDBに保存する。
func (c *Crawler) FetchChannel(ctx context.Context, channelID string) (*model.Channel, error) {
	info, err := c.client.GetConversationInfo(&slack.GetConversationInfoInput{
		ChannelID: channelID,
	})
	if err != nil {
		return nil, fmt.Errorf("GetConversationInfo: %w", err)
	}

	now := store.Now()
	ch := &model.Channel{
		ID:          info.ID,
		Name:        info.Name,
		Topic:       info.Topic.Value,
		Purpose:     info.Purpose.Value,
		IsPrivate:   info.IsPrivate,
		MemberCount: info.NumMembers,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := c.store.UpsertChannel(ctx, ch); err != nil {
		return nil, fmt.Errorf("UpsertChannel: %w", err)
	}
	return ch, nil
}

// CrawlMessages はチャンネルの全メッセージを取得しDBに保存する。
// oldest が空でなければそれ以降のメッセージのみ取得する（差分クロール）。
func (c *Crawler) CrawlMessages(ctx context.Context, channelID string, oldest string) (int, error) {
	total := 0
	cursor := ""

	for {
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}

		params := &slack.GetConversationHistoryParameters{
			ChannelID: channelID,
			Limit:     200,
			Cursor:    cursor,
		}
		if oldest != "" {
			params.Oldest = oldest
		}

		resp, err := c.client.GetConversationHistory(params)
		if err != nil {
			if rlErr, ok := err.(*slack.RateLimitedError); ok {
				c.logger.Info("レートリミット、待機中", "retry_after", rlErr.RetryAfter)
				time.Sleep(rlErr.RetryAfter)
				continue
			}
			return total, fmt.Errorf("GetConversationHistory: %w", err)
		}

		msgs, err := c.convertMessages(channelID, resp.Messages)
		if err != nil {
			return total, err
		}

		if err := c.store.UpsertMessages(ctx, msgs); err != nil {
			return total, fmt.Errorf("UpsertMessages: %w", err)
		}
		total += len(msgs)

		c.logger.Info("メッセージ取得", "channel", channelID, "batch", len(msgs), "total", total)
		c.progress("メッセージ取得中: %d件", total)

		if !resp.HasMore {
			break
		}
		cursor = resp.ResponseMetaData.NextCursor

		// Tier 3: 50 req/min → 1.2s interval
		time.Sleep(1200 * time.Millisecond)
	}

	return total, nil
}

// CrawlThreadReplies はチャンネル内のスレッド親メッセージに対して返信を取得しDBに保存する。
// parentMessages は reply_count > 0 のメッセージ一覧。
func (c *Crawler) CrawlThreadReplies(ctx context.Context, channelID string, parentMessages []*model.Message) (int, error) {
	total := 0
	for i, parent := range parentMessages {
		if parent.ReplyCount == 0 {
			continue
		}

		c.progress("スレッド返信取得中: %d/%dスレッド (%d件)", i+1, len(parentMessages), total)

		count, err := c.fetchReplies(ctx, channelID, parent.TS)
		if err != nil {
			return total, fmt.Errorf("fetchReplies(%s): %w", parent.TS, err)
		}
		total += count
	}
	return total, nil
}

// fetchReplies は1つのスレッドの返信を全件取得しDBに保存する。
func (c *Crawler) fetchReplies(ctx context.Context, channelID, threadTS string) (int, error) {
	total := 0
	cursor := ""

	for {
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}

		params := &slack.GetConversationRepliesParameters{
			ChannelID: channelID,
			Timestamp: threadTS,
			Limit:     200,
			Cursor:    cursor,
		}

		msgs, hasMore, nextCursor, err := c.client.GetConversationReplies(params)
		if err != nil {
			if rlErr, ok := err.(*slack.RateLimitedError); ok {
				c.logger.Info("レートリミット（replies）、待機中", "retry_after", rlErr.RetryAfter)
				time.Sleep(rlErr.RetryAfter)
				continue
			}
			return total, fmt.Errorf("GetConversationReplies: %w", err)
		}

		// 最初の要素はスレッド親なのでスキップ
		replies := make([]slack.Message, 0, len(msgs))
		for _, m := range msgs {
			if m.Timestamp != threadTS {
				replies = append(replies, m)
			}
		}

		converted, err := c.convertMessages(channelID, replies)
		if err != nil {
			return total, err
		}

		if err := c.store.UpsertMessages(ctx, converted); err != nil {
			return total, fmt.Errorf("UpsertMessages(replies): %w", err)
		}
		total += len(converted)

		if !hasMore {
			break
		}
		cursor = nextCursor

		// Tier 3: 50 req/min → 1.2s interval
		time.Sleep(1200 * time.Millisecond)
	}

	c.logger.Debug("スレッド返信取得", "thread_ts", threadTS, "replies", total)
	return total, nil
}

func (c *Crawler) convertMessages(channelID string, slackMsgs []slack.Message) ([]*model.Message, error) {
	now := store.Now()
	msgs := make([]*model.Message, 0, len(slackMsgs))

	for _, sm := range slackMsgs {
		raw, err := json.Marshal(sm)
		if err != nil {
			return nil, fmt.Errorf("json marshal: %w", err)
		}

		msgs = append(msgs, &model.Message{
			TS:         sm.Timestamp,
			ChannelID:  channelID,
			UserID:     sm.User,
			Text:       sm.Text,
			ThreadTS:   sm.ThreadTimestamp,
			ReplyCount: sm.ReplyCount,
			IsReply:    sm.ThreadTimestamp != "" && sm.ThreadTimestamp != sm.Timestamp,
			Subtype:    sm.SubType,
			RawJSON:    string(raw),
			CreatedAt:  tsToISO(sm.Timestamp),
			UpdatedAt:  now,
		})
	}
	return msgs, nil
}

// FetchUsers は指定ユーザーIDの情報を取得しDBに保存する。
func (c *Crawler) FetchUsers(ctx context.Context, userIDs []string) (int, error) {
	total := 0
	for _, uid := range userIDs {
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}

		info, err := c.client.GetUserInfo(uid)
		if err != nil {
			if rlErr, ok := err.(*slack.RateLimitedError); ok {
				c.logger.Info("レートリミット（users）、待機中", "retry_after", rlErr.RetryAfter)
				time.Sleep(rlErr.RetryAfter)
				// リトライ
				info, err = c.client.GetUserInfo(uid)
				if err != nil {
					c.logger.Warn("ユーザー情報取得失敗（スキップ）", "user_id", uid, "error", err)
					continue
				}
			} else {
				c.logger.Warn("ユーザー情報取得失敗（スキップ）", "user_id", uid, "error", err)
				continue
			}
		}

		u := &model.User{
			ID:          info.ID,
			Name:        info.Name,
			RealName:    info.RealName,
			DisplayName: info.Profile.DisplayName,
			IsBot:       info.IsBot,
			UpdatedAt:   store.Now(),
		}
		if err := c.store.UpsertUser(ctx, u); err != nil {
			return total, fmt.Errorf("UpsertUser(%s): %w", uid, err)
		}
		total++

		// Tier 4: 100 req/min → 0.6s interval
		time.Sleep(600 * time.Millisecond)
	}
	return total, nil
}

// SearchCrawl はSlack検索APIでメッセージを取得しDBに保存する。
// queryはSlack検索構文（例: "from:<@U07KF1QFLRY> after:2026-01-01"）。
func (c *Crawler) SearchCrawl(ctx context.Context, query string) (int, error) {
	total := 0
	page := 1

	for {
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}

		params := slack.SearchParameters{
			Sort:          "timestamp",
			SortDirection: "asc",
			Count:         100,
			Page:          page,
		}

		result, err := c.client.SearchMessages(query, params)
		if err != nil {
			if rlErr, ok := err.(*slack.RateLimitedError); ok {
				c.logger.Info("レートリミット（search）、待機中", "retry_after", rlErr.RetryAfter)
				time.Sleep(rlErr.RetryAfter)
				continue
			}
			return total, fmt.Errorf("SearchMessages: %w", err)
		}

		if len(result.Matches) == 0 {
			break
		}

		for _, match := range result.Matches {
			// チャンネル情報をUPSERT
			now := store.Now()
			ch := &model.Channel{
				ID:        match.Channel.ID,
				Name:      match.Channel.Name,
				IsPrivate: match.Channel.IsPrivate,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := c.store.UpsertChannel(ctx, ch); err != nil {
				return total, fmt.Errorf("UpsertChannel(%s): %w", match.Channel.ID, err)
			}

			// メッセージをUPSERT
			raw, _ := json.Marshal(match)
			msg := &model.Message{
				TS:        match.Timestamp,
				ChannelID: match.Channel.ID,
				UserID:    match.User,
				Text:      match.Text,
				RawJSON:   string(raw),
				CreatedAt: tsToISO(match.Timestamp),
				UpdatedAt: now,
			}
			if err := c.store.UpsertMessage(ctx, msg); err != nil {
				return total, fmt.Errorf("UpsertMessage: %w", err)
			}
			total++
		}

		c.progress("検索結果取得中: %d/%d件", total, result.Total)
		c.logger.Info("検索結果取得", "page", page, "batch", len(result.Matches), "total", total, "search_total", result.Total)

		if page >= result.Paging.Pages {
			break
		}
		page++

		// Tier 2: 20 req/min → 3s interval
		time.Sleep(3 * time.Second)
	}

	return total, nil
}

// tsToISO はSlackタイムスタンプ("1234567890.123456")をISO 8601に変換する。
func tsToISO(ts string) string {
	parts := strings.SplitN(ts, ".", 2)
	sec, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return ts
	}
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}

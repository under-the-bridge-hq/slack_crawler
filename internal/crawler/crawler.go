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

	"github.com/kaz-under-the-bridge/slack_crawler/internal/model"
	"github.com/kaz-under-the-bridge/slack_crawler/internal/store"
)

// Crawler はSlack APIからメッセージを取得しStoreに保存する。
type Crawler struct {
	client SlackClient
	store  *store.Store
	logger *slog.Logger
}

func New(client SlackClient, store *store.Store, logger *slog.Logger) *Crawler {
	return &Crawler{client: client, store: store, logger: logger}
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

		if !resp.HasMore {
			break
		}
		cursor = resp.ResponseMetaData.NextCursor

		// Tier 3: 50 req/min → 1.2s interval
		time.Sleep(1200 * time.Millisecond)
	}

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

// tsToISO はSlackタイムスタンプ("1234567890.123456")をISO 8601に変換する。
func tsToISO(ts string) string {
	parts := strings.SplitN(ts, ".", 2)
	sec, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return ts
	}
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}

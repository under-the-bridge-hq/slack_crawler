package crawler

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/slack-go/slack"

	"github.com/under-the-bridge-hq/slack_crawler/internal/store"
)

// mockSlackClient はテスト用のSlack APIモック。
type mockSlackClient struct {
	historyResp *slack.GetConversationHistoryResponse
	historyErr  error
	channelInfo *slack.Channel
	channelErr  error
}

func (m *mockSlackClient) GetConversationHistory(_ *slack.GetConversationHistoryParameters) (*slack.GetConversationHistoryResponse, error) {
	return m.historyResp, m.historyErr
}

func (m *mockSlackClient) GetConversationReplies(_ *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error) {
	return nil, false, "", nil
}

func (m *mockSlackClient) GetConversationInfo(_ *slack.GetConversationInfoInput) (*slack.Channel, error) {
	return m.channelInfo, m.channelErr
}

func setupTestCrawler(t *testing.T, client SlackClient) (*Crawler, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	cr := New(client, s, logger)
	return cr, s
}

func TestFetchChannel(t *testing.T) {
	mock := &mockSlackClient{
		channelInfo: &slack.Channel{
			GroupConversation: slack.GroupConversation{
				Conversation: slack.Conversation{
					ID:        "C001",
					IsPrivate: false,
				},
				Name:    "general",
				Topic:   slack.Topic{Value: "一般チャンネル"},
				Purpose: slack.Purpose{Value: "雑談用"},
			},
		},
	}

	cr, s := setupTestCrawler(t, mock)
	ctx := context.Background()

	ch, err := cr.FetchChannel(ctx, "C001")
	if err != nil {
		t.Fatalf("FetchChannel: %v", err)
	}
	if ch.Name != "general" {
		t.Errorf("want general, got %s", ch.Name)
	}

	channels, _ := s.ListChannels(ctx)
	if len(channels) != 1 {
		t.Fatalf("want 1 channel in DB, got %d", len(channels))
	}
}

func TestCrawlMessages(t *testing.T) {
	mock := &mockSlackClient{
		channelInfo: &slack.Channel{
			GroupConversation: slack.GroupConversation{
				Conversation: slack.Conversation{ID: "C001"},
				Name:         "test",
			},
		},
		historyResp: &slack.GetConversationHistoryResponse{
			HasMore: false,
			Messages: []slack.Message{
				{Msg: slack.Msg{Timestamp: "1700000001.000001", User: "U001", Text: "hello"}},
				{Msg: slack.Msg{Timestamp: "1700000002.000001", User: "U002", Text: "world"}},
			},
		},
	}

	cr, s := setupTestCrawler(t, mock)
	ctx := context.Background()

	// チャンネルを先に保存
	cr.FetchChannel(ctx, "C001")

	total, err := cr.CrawlMessages(ctx, "C001", "")
	if err != nil {
		t.Fatalf("CrawlMessages: %v", err)
	}
	if total != 2 {
		t.Errorf("want 2 messages, got %d", total)
	}

	count, _ := s.CountMessages(ctx, "C001")
	if count != 2 {
		t.Errorf("want 2 in DB, got %d", count)
	}
}

func TestCrawlMessages_WithOldest(t *testing.T) {
	called := false
	mock := &mockSlackClient{
		channelInfo: &slack.Channel{
			GroupConversation: slack.GroupConversation{
				Conversation: slack.Conversation{ID: "C001"},
				Name:         "test",
			},
		},
		historyResp: &slack.GetConversationHistoryResponse{
			HasMore:  false,
			Messages: []slack.Message{},
		},
	}

	cr, _ := setupTestCrawler(t, mock)
	ctx := context.Background()
	cr.FetchChannel(ctx, "C001")

	// oldestパラメータ付きで呼ぶ（レスポンスは空でOK）
	_ = called
	_, err := cr.CrawlMessages(ctx, "C001", "1700000000.000000")
	if err != nil {
		t.Fatalf("CrawlMessages with oldest: %v", err)
	}
}

func TestTsToISO(t *testing.T) {
	tests := []struct {
		ts   string
		want string
	}{
		{"1700000000.000001", "2023-11-14T22:13:20Z"},
		{"invalid", "invalid"},
	}
	for _, tt := range tests {
		got := tsToISO(tt.ts)
		if got != tt.want {
			t.Errorf("tsToISO(%q) = %q, want %q", tt.ts, got, tt.want)
		}
	}
}

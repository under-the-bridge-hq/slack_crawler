package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/under-the-bridge-hq/slack_crawler/internal/model"
)

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNew_CreatesDBFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("DBファイルが作成されていない")
	}
}

func TestUpsertChannel(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	now := Now()

	ch := &model.Channel{
		ID:          "C001",
		Name:        "general",
		Topic:       "一般",
		Purpose:     "雑談",
		IsPrivate:   false,
		MemberCount: 10,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.UpsertChannel(ctx, ch); err != nil {
		t.Fatalf("UpsertChannel insert: %v", err)
	}

	// 同じIDでUPDATE
	ch.Name = "general-renamed"
	ch.MemberCount = 20
	if err := s.UpsertChannel(ctx, ch); err != nil {
		t.Fatalf("UpsertChannel update: %v", err)
	}

	channels, err := s.ListChannels(ctx)
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("want 1 channel, got %d", len(channels))
	}
	if channels[0].Name != "general-renamed" {
		t.Errorf("want name general-renamed, got %s", channels[0].Name)
	}
}

func TestUpsertMessage_AndCount(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	now := Now()

	ch := &model.Channel{ID: "C001", Name: "test", CreatedAt: now, UpdatedAt: now}
	if err := s.UpsertChannel(ctx, ch); err != nil {
		t.Fatalf("UpsertChannel: %v", err)
	}

	msg := &model.Message{
		TS:        "1234567890.000100",
		ChannelID: "C001",
		UserID:    "U001",
		Text:      "hello",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.UpsertMessage(ctx, msg); err != nil {
		t.Fatalf("UpsertMessage: %v", err)
	}

	count, err := s.CountMessages(ctx, "C001")
	if err != nil {
		t.Fatalf("CountMessages: %v", err)
	}
	if count != 1 {
		t.Errorf("want 1 message, got %d", count)
	}

	// 同じメッセージをUPDATE
	msg.Text = "hello updated"
	if err := s.UpsertMessage(ctx, msg); err != nil {
		t.Fatalf("UpsertMessage update: %v", err)
	}

	count, err = s.CountMessages(ctx, "C001")
	if err != nil {
		t.Fatalf("CountMessages: %v", err)
	}
	if count != 1 {
		t.Errorf("want 1 message after upsert, got %d", count)
	}
}

func TestUpsertMessages_Batch(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	now := Now()

	ch := &model.Channel{ID: "C001", Name: "test", CreatedAt: now, UpdatedAt: now}
	if err := s.UpsertChannel(ctx, ch); err != nil {
		t.Fatalf("UpsertChannel: %v", err)
	}

	msgs := []*model.Message{
		{TS: "1000.001", ChannelID: "C001", UserID: "U001", Text: "msg1", CreatedAt: now, UpdatedAt: now},
		{TS: "1000.002", ChannelID: "C001", UserID: "U002", Text: "msg2", CreatedAt: now, UpdatedAt: now},
		{TS: "1000.003", ChannelID: "C001", UserID: "U001", Text: "msg3", CreatedAt: now, UpdatedAt: now},
	}
	if err := s.UpsertMessages(ctx, msgs); err != nil {
		t.Fatalf("UpsertMessages: %v", err)
	}

	count, err := s.CountMessages(ctx, "C001")
	if err != nil {
		t.Fatalf("CountMessages: %v", err)
	}
	if count != 3 {
		t.Errorf("want 3 messages, got %d", count)
	}
}

func TestGetLatestTS(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	now := Now()

	// 空の場合
	ts, err := s.GetLatestTS(ctx, "C001")
	if err != nil {
		t.Fatalf("GetLatestTS: %v", err)
	}
	if ts != "" {
		t.Errorf("want empty ts, got %q", ts)
	}

	ch := &model.Channel{ID: "C001", Name: "test", CreatedAt: now, UpdatedAt: now}
	s.UpsertChannel(ctx, ch)

	msgs := []*model.Message{
		{TS: "1000.001", ChannelID: "C001", Text: "a", CreatedAt: now, UpdatedAt: now},
		{TS: "2000.001", ChannelID: "C001", Text: "b", CreatedAt: now, UpdatedAt: now},
		{TS: "1500.001", ChannelID: "C001", Text: "c", CreatedAt: now, UpdatedAt: now},
	}
	s.UpsertMessages(ctx, msgs)

	ts, err = s.GetLatestTS(ctx, "C001")
	if err != nil {
		t.Fatalf("GetLatestTS: %v", err)
	}
	if ts != "2000.001" {
		t.Errorf("want 2000.001, got %s", ts)
	}
}

func TestListChannels_MultipleChannels(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	now := Now()

	channels := []*model.Channel{
		{ID: "C001", Name: "beta", CreatedAt: now, UpdatedAt: now},
		{ID: "C002", Name: "alpha", CreatedAt: now, UpdatedAt: now},
	}
	for _, ch := range channels {
		if err := s.UpsertChannel(ctx, ch); err != nil {
			t.Fatalf("UpsertChannel: %v", err)
		}
	}

	result, err := s.ListChannels(ctx)
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("want 2 channels, got %d", len(result))
	}
	if result[0].Name != "alpha" {
		t.Errorf("want alpha first, got %s", result[0].Name)
	}
}

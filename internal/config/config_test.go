package config

import (
	"os"
	"testing"
)

func TestValidate_MissingToken(t *testing.T) {
	cfg := &Config{}
	if err := cfg.Validate(); err == nil {
		t.Error("want error for missing token")
	}
}

func TestValidate_WithBotToken(t *testing.T) {
	cfg := &Config{SlackBotToken: "xoxb-test"}
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_WithUserToken(t *testing.T) {
	cfg := &Config{SlackUserToken: "xoxp-test"}
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSlackToken_UserTokenPriority(t *testing.T) {
	cfg := &Config{
		SlackBotToken:  "xoxb-test",
		SlackUserToken: "xoxp-test",
	}
	if got := cfg.SlackToken(); got != "xoxp-test" {
		t.Errorf("want user token, got %s", got)
	}
}

func TestSlackToken_FallbackToBot(t *testing.T) {
	cfg := &Config{SlackBotToken: "xoxb-test"}
	if got := cfg.SlackToken(); got != "xoxb-test" {
		t.Errorf("want bot token, got %s", got)
	}
}

func TestValidateForCrawl_MissingChannels(t *testing.T) {
	cfg := &Config{SlackBotToken: "xoxb-test"}
	if err := cfg.ValidateForCrawl(); err == nil {
		t.Error("want error for missing channels")
	}
}

func TestValidateForCrawl_OK(t *testing.T) {
	cfg := &Config{
		SlackUserToken: "xoxp-test",
		Channels:       []ChannelEntry{{ID: "C001", Name: "test"}},
	}
	if err := cfg.ValidateForCrawl(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestChannelIDs(t *testing.T) {
	cfg := &Config{
		Channels: []ChannelEntry{
			{ID: "C001", Name: "general"},
			{ID: "C002", Name: "random"},
		},
	}
	ids := cfg.ChannelIDs()
	if len(ids) != 2 || ids[0] != "C001" || ids[1] != "C002" {
		t.Errorf("want [C001 C002], got %v", ids)
	}
}

func TestLoadChannelsYAML(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/channels.yml"
	data := []byte("channels:\n  - id: C001\n    name: general\n  - id: C002\n    name: random\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	channels, err := loadChannelsYAML(path)
	if err != nil {
		t.Fatalf("loadChannelsYAML: %v", err)
	}
	if len(channels) != 2 {
		t.Fatalf("want 2 channels, got %d", len(channels))
	}
	if channels[0].ID != "C001" || channels[0].Name != "general" {
		t.Errorf("want C001/general, got %s/%s", channels[0].ID, channels[0].Name)
	}
}

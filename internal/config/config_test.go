package config

import "testing"

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
		ChannelIDs:     []string{"C001"},
	}
	if err := cfg.ValidateForCrawl(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

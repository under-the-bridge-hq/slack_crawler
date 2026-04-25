package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// ChannelEntry はチャンネル設定の1エントリ。
type ChannelEntry struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

// Config はアプリケーション設定を保持する。
type Config struct {
	SlackBotToken  string         `mapstructure:"slack_bot_token"`
	SlackUserToken string         `mapstructure:"slack_user_token"`
	Channels       []ChannelEntry // channels.yml または SLACK_CHANNEL_IDS から
	DBPath         string         `mapstructure:"db_path"`
	LogLevel       string         `mapstructure:"log_level"`
}

// ChannelIDs はチャンネルIDのスライスを返す（後方互換）。
func (c *Config) ChannelIDs() []string {
	ids := make([]string, len(c.Channels))
	for i, ch := range c.Channels {
		ids[i] = ch.ID
	}
	return ids
}

// channelsFile はchannels.ymlの構造。
type channelsFile struct {
	Channels []ChannelEntry `yaml:"channels"`
}

// Load は環境変数・.envファイル・channels.ymlから設定を読み込む。
func Load() (*Config, error) {
	v := viper.New()

	v.SetConfigName(".env")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	v.ReadInConfig() // .envが無くてもエラーにしない

	v.AutomaticEnv()

	v.SetDefault("db_path", "data/slack.db")
	v.SetDefault("log_level", "info")

	var cfg Config
	cfg.SlackBotToken = v.GetString("slack_bot_token")
	cfg.SlackUserToken = v.GetString("slack_user_token")
	cfg.DBPath = v.GetString("db_path")
	cfg.LogLevel = v.GetString("log_level")

	// channels.yml を優先で読み込み
	if channels, err := loadChannelsYAML("channels.yml"); err == nil && len(channels) > 0 {
		cfg.Channels = channels
	} else {
		// フォールバック: SLACK_CHANNEL_IDS（カンマ区切り）
		raw := v.GetString("slack_channel_ids")
		if raw != "" {
			for _, id := range strings.Split(raw, ",") {
				id = strings.TrimSpace(id)
				if id != "" {
					cfg.Channels = append(cfg.Channels, ChannelEntry{ID: id})
				}
			}
		}
	}

	return &cfg, nil
}

func loadChannelsYAML(path string) ([]ChannelEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f channelsFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("channels.yml parse: %w", err)
	}
	return f.Channels, nil
}

// SlackToken はUser Tokenを優先し、なければBot Tokenを返す。
func (c *Config) SlackToken() string {
	if c.SlackUserToken != "" {
		return c.SlackUserToken
	}
	return c.SlackBotToken
}

// Validate は必須設定の存在を確認する。
func (c *Config) Validate() error {
	if c.SlackToken() == "" {
		return fmt.Errorf("SLACK_USER_TOKEN または SLACK_BOT_TOKEN が設定されていません")
	}
	return nil
}

// ValidateForCrawl はクロール実行時に必要な設定を確認する。
func (c *Config) ValidateForCrawl() error {
	if err := c.Validate(); err != nil {
		return err
	}
	if len(c.Channels) == 0 {
		return fmt.Errorf("channels.yml または SLACK_CHANNEL_IDS が設定されていません")
	}
	return nil
}

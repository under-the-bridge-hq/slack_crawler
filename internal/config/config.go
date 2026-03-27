package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config はアプリケーション設定を保持する。
type Config struct {
	SlackBotToken  string   `mapstructure:"slack_bot_token"`
	SlackUserToken string   `mapstructure:"slack_user_token"`
	ChannelIDs     []string `mapstructure:"slack_channel_ids"`
	DBPath         string   `mapstructure:"db_path"`
	LogLevel       string   `mapstructure:"log_level"`
}

// Load は環境変数・.envファイルから設定を読み込む。
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

	// SLACK_CHANNEL_IDS はカンマ区切りで複数指定
	raw := v.GetString("slack_channel_ids")
	if raw != "" {
		for _, id := range strings.Split(raw, ",") {
			id = strings.TrimSpace(id)
			if id != "" {
				cfg.ChannelIDs = append(cfg.ChannelIDs, id)
			}
		}
	}

	return &cfg, nil
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
	if len(c.ChannelIDs) == 0 {
		return fmt.Errorf("SLACK_CHANNEL_IDS が設定されていません")
	}
	return nil
}

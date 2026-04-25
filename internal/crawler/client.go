package crawler

import (
	"github.com/slack-go/slack"
)

// SlackClient はSlack APIの抽象化インターフェース。テスト時にモック差し替え可能。
type SlackClient interface {
	GetConversationHistory(params *slack.GetConversationHistoryParameters) (*slack.GetConversationHistoryResponse, error)
	GetConversationReplies(params *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error)
	GetConversationInfo(input *slack.GetConversationInfoInput) (*slack.Channel, error)
	GetUserInfo(userID string) (*slack.User, error)
	SearchMessages(query string, params slack.SearchParameters) (*slack.SearchMessages, error)
}

// slackClientWrapper はslack.Clientをインターフェースに適合させるラッパー。
type slackClientWrapper struct {
	client *slack.Client
}

func NewSlackClient(token string) SlackClient {
	return &slackClientWrapper{client: slack.New(token)}
}

func (w *slackClientWrapper) GetConversationHistory(params *slack.GetConversationHistoryParameters) (*slack.GetConversationHistoryResponse, error) {
	return w.client.GetConversationHistory(params)
}

func (w *slackClientWrapper) GetConversationReplies(params *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error) {
	return w.client.GetConversationReplies(params)
}

func (w *slackClientWrapper) GetConversationInfo(input *slack.GetConversationInfoInput) (*slack.Channel, error) {
	return w.client.GetConversationInfo(input)
}

func (w *slackClientWrapper) GetUserInfo(userID string) (*slack.User, error) {
	return w.client.GetUserInfo(userID)
}

func (w *slackClientWrapper) SearchMessages(query string, params slack.SearchParameters) (*slack.SearchMessages, error) {
	return w.client.SearchMessages(query, params)
}

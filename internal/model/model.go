package model

// Channel はSlackチャンネルのメタ情報を表す。
type Channel struct {
	ID          string `db:"id"`
	Name        string `db:"name"`
	Topic       string `db:"topic"`
	Purpose     string `db:"purpose"`
	IsPrivate   bool   `db:"is_private"`
	MemberCount int    `db:"member_count"`
	CreatedAt   string `db:"created_at"`
	UpdatedAt   string `db:"updated_at"`
}

// User はSlackユーザー情報を表す。
type User struct {
	ID          string `db:"id"`
	Name        string `db:"name"`
	RealName    string `db:"real_name"`
	DisplayName string `db:"display_name"`
	IsBot       bool   `db:"is_bot"`
	UpdatedAt   string `db:"updated_at"`
}

// CrawlLog はクロール実行履歴を表す。
type CrawlLog struct {
	ID              int64  `db:"id"`
	ChannelID       string `db:"channel_id"`
	StartedAt       string `db:"started_at"`
	FinishedAt      string `db:"finished_at"`
	MessagesFetched int    `db:"messages_fetched"`
	ThreadsFetched  int    `db:"threads_fetched"`
	Status          string `db:"status"`
	Error           string `db:"error"`
}

// Message はSlackメッセージを表す。スレッド返信も同一構造。
type Message struct {
	TS         string `db:"ts"`
	ChannelID  string `db:"channel_id"`
	UserID     string `db:"user_id"`
	Text       string `db:"text"`
	ThreadTS   string `db:"thread_ts"`
	ReplyCount int    `db:"reply_count"`
	IsReply    bool   `db:"is_reply"`
	Subtype    string `db:"subtype"`
	RawJSON    string `db:"raw_json"`
	CreatedAt  string `db:"created_at"`
	UpdatedAt  string `db:"updated_at"`
}

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

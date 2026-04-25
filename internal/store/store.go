package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"github.com/under-the-bridge-hq/slack_crawler/internal/model"
)

// Store はSQLiteへのアクセスを提供する。
type Store struct {
	db *sqlx.DB
}

// New はSQLite接続を開き、スキーマを初期化してStoreを返す。
func New(dbPath string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_foreign_keys=ON&_busy_timeout=5000", dbPath)
	db, err := sqlx.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close はDB接続を閉じる。
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS channels (
			id           TEXT PRIMARY KEY,
			name         TEXT NOT NULL,
			topic        TEXT DEFAULT '',
			purpose      TEXT DEFAULT '',
			is_private   INTEGER DEFAULT 0,
			member_count INTEGER DEFAULT 0,
			created_at   TEXT NOT NULL,
			updated_at   TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			ts          TEXT NOT NULL,
			channel_id  TEXT NOT NULL,
			user_id     TEXT DEFAULT '',
			text        TEXT DEFAULT '',
			thread_ts   TEXT DEFAULT '',
			reply_count INTEGER DEFAULT 0,
			is_reply    INTEGER DEFAULT 0,
			subtype     TEXT DEFAULT '',
			raw_json    TEXT DEFAULT '',
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL,
			PRIMARY KEY (ts, channel_id),
			FOREIGN KEY (channel_id) REFERENCES channels(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_channel_ts
			ON messages(channel_id, ts DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_thread_ts
			ON messages(thread_ts) WHERE thread_ts != ''`,
		`CREATE INDEX IF NOT EXISTS idx_messages_user_id
			ON messages(user_id)`,
		`CREATE TABLE IF NOT EXISTS users (
			id           TEXT PRIMARY KEY,
			name         TEXT NOT NULL,
			real_name    TEXT DEFAULT '',
			display_name TEXT DEFAULT '',
			is_bot       INTEGER DEFAULT 0,
			updated_at   TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS crawl_logs (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			channel_id       TEXT NOT NULL,
			started_at       TEXT NOT NULL,
			finished_at      TEXT DEFAULT '',
			messages_fetched INTEGER DEFAULT 0,
			threads_fetched  INTEGER DEFAULT 0,
			status           TEXT DEFAULT 'running',
			error            TEXT DEFAULT '',
			FOREIGN KEY (channel_id) REFERENCES channels(id)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:40], err)
		}
	}
	return nil
}

// UpsertChannel はチャンネル情報をINSERT or UPDATEする。
func (s *Store) UpsertChannel(ctx context.Context, ch *model.Channel) error {
	query := `INSERT INTO channels (id, name, topic, purpose, is_private, member_count, created_at, updated_at)
		VALUES (:id, :name, :topic, :purpose, :is_private, :member_count, :created_at, :updated_at)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			topic = excluded.topic,
			purpose = excluded.purpose,
			is_private = excluded.is_private,
			member_count = excluded.member_count,
			updated_at = excluded.updated_at`
	_, err := s.db.NamedExecContext(ctx, query, ch)
	return err
}

// UpsertMessage はメッセージをINSERT or UPDATEする。
func (s *Store) UpsertMessage(ctx context.Context, msg *model.Message) error {
	query := `INSERT INTO messages (ts, channel_id, user_id, text, thread_ts, reply_count, is_reply, subtype, raw_json, created_at, updated_at)
		VALUES (:ts, :channel_id, :user_id, :text, :thread_ts, :reply_count, :is_reply, :subtype, :raw_json, :created_at, :updated_at)
		ON CONFLICT(ts, channel_id) DO UPDATE SET
			user_id = excluded.user_id,
			text = excluded.text,
			thread_ts = excluded.thread_ts,
			reply_count = excluded.reply_count,
			is_reply = excluded.is_reply,
			subtype = excluded.subtype,
			raw_json = excluded.raw_json,
			updated_at = excluded.updated_at`
	_, err := s.db.NamedExecContext(ctx, query, msg)
	return err
}

// UpsertMessages はメッセージを一括UPSERTする。
func (s *Store) UpsertMessages(ctx context.Context, msgs []*model.Message) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	query := `INSERT INTO messages (ts, channel_id, user_id, text, thread_ts, reply_count, is_reply, subtype, raw_json, created_at, updated_at)
		VALUES (:ts, :channel_id, :user_id, :text, :thread_ts, :reply_count, :is_reply, :subtype, :raw_json, :created_at, :updated_at)
		ON CONFLICT(ts, channel_id) DO UPDATE SET
			user_id = excluded.user_id,
			text = excluded.text,
			thread_ts = excluded.thread_ts,
			reply_count = excluded.reply_count,
			is_reply = excluded.is_reply,
			subtype = excluded.subtype,
			raw_json = excluded.raw_json,
			updated_at = excluded.updated_at`

	for _, msg := range msgs {
		if _, err := tx.NamedExecContext(ctx, query, msg); err != nil {
			return fmt.Errorf("upsert message %s: %w", msg.TS, err)
		}
	}
	return tx.Commit()
}

// GetLatestTS は指定チャンネルの最新メッセージTSを返す（差分クロール用）。
func (s *Store) GetLatestTS(ctx context.Context, channelID string) (string, error) {
	var ts string
	err := s.db.GetContext(ctx, &ts,
		`SELECT COALESCE(MAX(ts), '') FROM messages WHERE channel_id = ?`, channelID)
	return ts, err
}

// CountMessages は指定チャンネルのメッセージ数を返す。
func (s *Store) CountMessages(ctx context.Context, channelID string) (int, error) {
	var count int
	err := s.db.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM messages WHERE channel_id = ?`, channelID)
	return count, err
}

// ListChannels は保存済みの全チャンネルを返す。
func (s *Store) ListChannels(ctx context.Context) ([]model.Channel, error) {
	var channels []model.Channel
	err := s.db.SelectContext(ctx, &channels,
		`SELECT id, name, topic, purpose, is_private, member_count, created_at, updated_at FROM channels ORDER BY name`)
	return channels, err
}

// UpsertUser はユーザー情報をINSERT or UPDATEする。
func (s *Store) UpsertUser(ctx context.Context, u *model.User) error {
	query := `INSERT INTO users (id, name, real_name, display_name, is_bot, updated_at)
		VALUES (:id, :name, :real_name, :display_name, :is_bot, :updated_at)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			real_name = excluded.real_name,
			display_name = excluded.display_name,
			is_bot = excluded.is_bot,
			updated_at = excluded.updated_at`
	_, err := s.db.NamedExecContext(ctx, query, u)
	return err
}

// GetDistinctUserIDs はmessagesテーブルから未登録のユーザーIDを返す。
func (s *Store) GetDistinctUserIDs(ctx context.Context) ([]string, error) {
	var ids []string
	err := s.db.SelectContext(ctx, &ids,
		`SELECT DISTINCT m.user_id FROM messages m
		 LEFT JOIN users u ON m.user_id = u.id
		 WHERE m.user_id != '' AND u.id IS NULL`)
	return ids, err
}

// StartCrawlLog はクロール開始を記録しログIDを返す。
func (s *Store) StartCrawlLog(ctx context.Context, channelID string) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO crawl_logs (channel_id, started_at, status) VALUES (?, ?, 'running')`,
		channelID, Now())
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// CompleteCrawlLog はクロール完了を記録する。
func (s *Store) CompleteCrawlLog(ctx context.Context, logID int64, msgCount, threadCount int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE crawl_logs SET finished_at = ?, messages_fetched = ?, threads_fetched = ?, status = 'completed' WHERE id = ?`,
		Now(), msgCount, threadCount, logID)
	return err
}

// FailCrawlLog はクロール失敗を記録する。
func (s *Store) FailCrawlLog(ctx context.Context, logID int64, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE crawl_logs SET finished_at = ?, status = 'failed', error = ? WHERE id = ?`,
		Now(), errMsg, logID)
	return err
}

// GetThreadParents はreply_count > 0 のメッセージ（スレッド親）を返す。
func (s *Store) GetThreadParents(ctx context.Context, channelID string) ([]*model.Message, error) {
	var msgs []*model.Message
	err := s.db.SelectContext(ctx, &msgs,
		`SELECT ts, channel_id, user_id, text, thread_ts, reply_count, is_reply, subtype, raw_json, created_at, updated_at
		 FROM messages WHERE channel_id = ? AND reply_count > 0 ORDER BY ts`, channelID)
	return msgs, err
}

// GetStoredReplyCount は保存済みスレッド親のreply_countをmap[ts]intで返す。
func (s *Store) GetStoredReplyCount(ctx context.Context, channelID string) (map[string]int, error) {
	type row struct {
		TS         string `db:"ts"`
		ReplyCount int    `db:"reply_count"`
	}
	var rows []row
	err := s.db.SelectContext(ctx, &rows,
		`SELECT ts, reply_count FROM messages WHERE channel_id = ? AND reply_count > 0`, channelID)
	if err != nil {
		return nil, err
	}
	m := make(map[string]int, len(rows))
	for _, r := range rows {
		m[r.TS] = r.ReplyCount
	}
	return m, nil
}

// RawQuery は任意のSELECTクエリを実行し、カラム名と行データを返す。
func (s *Store) RawQuery(ctx context.Context, query string) ([]string, [][]string, error) {
	rows, err := s.db.QueryxContext(ctx, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}

	var results [][]string
	for rows.Next() {
		cols, err := rows.SliceScan()
		if err != nil {
			return nil, nil, err
		}
		row := make([]string, len(cols))
		for i, v := range cols {
			if v == nil {
				row[i] = "NULL"
			} else {
				row[i] = fmt.Sprintf("%v", v)
			}
		}
		results = append(results, row)
	}
	return columns, results, rows.Err()
}

// Now はISO 8601形式の現在時刻文字列を返すヘルパー。
func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

# SQLiteスキーマ設計

## テーブル定義

### channels

チャンネルのメタ情報を保持。

```sql
CREATE TABLE IF NOT EXISTS channels (
    id          TEXT PRIMARY KEY,   -- Slack Channel ID (C0XXXXXXX)
    name        TEXT NOT NULL,
    topic       TEXT DEFAULT '',
    purpose     TEXT DEFAULT '',
    is_private  INTEGER DEFAULT 0,
    member_count INTEGER DEFAULT 0,
    created_at  TEXT NOT NULL,      -- ISO 8601
    updated_at  TEXT NOT NULL       -- ISO 8601
);
```

### messages

メッセージ本文とメタ情報。スレッド返信も同一テーブルに格納。

```sql
CREATE TABLE IF NOT EXISTS messages (
    ts          TEXT NOT NULL,       -- Slackタイムスタンプ（メッセージID）
    channel_id  TEXT NOT NULL,
    user_id     TEXT DEFAULT '',
    text        TEXT DEFAULT '',
    thread_ts   TEXT DEFAULT '',     -- スレッド親のts（空ならトップレベル）
    reply_count INTEGER DEFAULT 0,
    is_reply    INTEGER DEFAULT 0,   -- スレッド返信かどうか
    subtype     TEXT DEFAULT '',     -- メッセージサブタイプ
    raw_json    TEXT DEFAULT '',     -- 元のJSON（将来の拡張用）
    created_at  TEXT NOT NULL,       -- ISO 8601（ts変換）
    updated_at  TEXT NOT NULL,       -- ISO 8601
    PRIMARY KEY (ts, channel_id),
    FOREIGN KEY (channel_id) REFERENCES channels(id)
);

CREATE INDEX IF NOT EXISTS idx_messages_channel_ts
    ON messages(channel_id, ts DESC);

CREATE INDEX IF NOT EXISTS idx_messages_thread_ts
    ON messages(thread_ts)
    WHERE thread_ts != '';

CREATE INDEX IF NOT EXISTS idx_messages_user_id
    ON messages(user_id);
```

### users

ユーザー情報。クロール後に未登録ユーザーを自動取得。

```sql
CREATE TABLE IF NOT EXISTS users (
    id          TEXT PRIMARY KEY,    -- Slack User ID (U0XXXXXXX)
    name        TEXT NOT NULL,
    real_name   TEXT DEFAULT '',
    display_name TEXT DEFAULT '',
    is_bot      INTEGER DEFAULT 0,
    updated_at  TEXT NOT NULL        -- ISO 8601
);
```

### crawl_logs

クロール実行履歴。crawlコマンド実行時に自動記録。

```sql
CREATE TABLE IF NOT EXISTS crawl_logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id  TEXT NOT NULL,
    started_at  TEXT NOT NULL,       -- ISO 8601
    finished_at TEXT,                -- ISO 8601
    messages_fetched INTEGER DEFAULT 0,
    threads_fetched  INTEGER DEFAULT 0,
    status      TEXT DEFAULT 'running', -- running / completed / failed
    error       TEXT DEFAULT '',
    FOREIGN KEY (channel_id) REFERENCES channels(id)
);
```

## 初期化

```sql
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
PRAGMA busy_timeout=5000;
```

## よくある分析クエリ例

```sql
-- チャンネル別メッセージ数
SELECT c.name, COUNT(*) as msg_count
FROM messages m JOIN channels c ON m.channel_id = c.id
GROUP BY c.name ORDER BY msg_count DESC;

-- 特定チャンネルの日別投稿数
SELECT DATE(created_at) as date, COUNT(*) as count
FROM messages WHERE channel_id = 'C0XXXXXXX'
GROUP BY date ORDER BY date;

-- スレッドの多い投稿（ホットトピック）
SELECT ts, text, reply_count
FROM messages WHERE channel_id = 'C0XXXXXXX' AND reply_count > 0
ORDER BY reply_count DESC LIMIT 20;

-- ユーザー別投稿数ランキング
SELECT u.display_name, COUNT(*) as count
FROM messages m JOIN users u ON m.user_id = u.id
WHERE m.channel_id = 'C0XXXXXXX'
GROUP BY u.id ORDER BY count DESC;
```

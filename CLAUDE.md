# CLAUDE.md

## プロジェクト概要

Slack APIを利用して指定チャンネルの全メッセージ（スレッド含む）をクロールし、SQLiteにローカル保存するCLIツール。保存したデータはSQLで自由に分析できる。

## 技術スタック

- **言語**: Go 1.23+
- **Slack SDK**: [slack-go/slack](https://github.com/slack-go/slack)
- **DB**: SQLite（[modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) — CGO不要の純Go実装）
- **CLI**: [cobra](https://github.com/spf13/cobra) + [viper](https://github.com/spf13/viper)
- **ログ**: [slog](https://pkg.go.dev/log/slog)（標準ライブラリ）

## プロジェクト構造

```
slack_crawler/
├── CLAUDE.md                 # Claude Code向けガイダンス
├── README.md                 # クイックスタートガイド
├── .claude/
│   └── settings.json         # Claude Codeフック設定
├── docs/
│   ├── architecture.md       # アーキテクチャ概要・データフロー
│   ├── implementation-plan.md # MVP計画・進捗管理
│   ├── schema.md             # SQLiteスキーマ設計
│   └── slack-api.md          # Slack API利用方針・レートリミット対策
├── cmd/
│   └── slack-crawler/
│       └── main.go           # エントリーポイント
├── internal/
│   ├── config/               # 設定管理（env/flag/config file）
│   ├── crawler/              # Slack APIクロール制御
│   ├── store/                # SQLiteアクセス層
│   └── model/                # データモデル定義
├── data/                     # SQLiteファイル格納（gitignore対象）
├── .env.example              # 環境変数テンプレート
├── .gitignore
├── go.mod
└── go.sum
```

## モジュール設計

| パッケージ | 責務 |
|-----------|------|
| `cmd/slack-crawler` | CLIエントリーポイント。cobra/viperによるサブコマンド定義 |
| `internal/config` | 環境変数・設定ファイルの読み込みと検証 |
| `internal/crawler` | Slack API呼び出し、ページネーション、レートリミット制御 |
| `internal/store` | SQLiteへのメッセージ保存・検索。スキーマ管理 |
| `internal/model` | Message, Channel, Thread等のデータ構造定義 |

## CLIサブコマンド（予定）

```bash
# チャンネル一覧を取得
slack-crawler channels

# 指定チャンネルのメッセージをクロール
slack-crawler crawl --channel <CHANNEL_ID> [--since <DATE>]

# 保存済みデータの統計を表示
slack-crawler stats

# SQLiteに直接クエリを実行
slack-crawler query "SELECT * FROM messages WHERE channel_id = 'C0XXX' LIMIT 10"
```

## 開発ガイドライン

### コーディング規約

- `go fmt` / `go vet` を常に通す
- `internal/` パッケージで外部公開を制御
- エラーは `fmt.Errorf("context: %w", err)` でラップ
- slogでの構造化ログ出力（JSON形式）
- テストは `*_test.go` でテーブル駆動テストを基本とする

### Slack API利用方針

- Bot Token（`xoxb-`）を使用
- 必要なスコープ: `channels:history`, `channels:read`, `groups:history`, `groups:read`, `users:read`
- レートリミット: Tier 3 API（`conversations.history`, `conversations.replies`）は1分あたり50リクエスト
- ページネーション: cursorベースで全件取得
- 429レスポンス時は `Retry-After` ヘッダーに従う

### SQLite方針

- WALモード有効化（書き込み性能向上）
- スキーママイグレーションはアプリ起動時に自動実行
- データファイルは `data/` ディレクトリに配置（gitignore対象）

## 環境変数

| 変数名 | 説明 | 必須 |
|--------|------|------|
| `SLACK_BOT_TOKEN` | Slack Bot Token (`xoxb-`) | ✓ |
| `SLACK_CHANNEL_IDS` | クロール対象チャンネルID（カンマ区切り） | ✓ |
| `DB_PATH` | SQLiteファイルパス（デフォルト: `data/slack.db`） | |
| `LOG_LEVEL` | ログレベル（debug/info/warn/error、デフォルト: info） | |

## ビルド・実行

```bash
# ビルド
go build -o slack-crawler ./cmd/slack-crawler

# 実行
cp .env.example .env
# .envを編集してSLACK_BOT_TOKENを設定
./slack-crawler crawl --channel C0XXXXXXX
```

## 関連ドキュメント

- [アーキテクチャ概要](docs/architecture.md)
- [MVP計画](docs/implementation-plan.md)
- [SQLiteスキーマ設計](docs/schema.md)
- [Slack API利用方針](docs/slack-api.md)

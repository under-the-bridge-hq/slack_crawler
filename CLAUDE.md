# CLAUDE.md

## プロジェクト概要

Slack APIを利用して指定チャンネルの全メッセージ（スレッド含む）をクロールし、SQLiteにローカル保存するCLIツール。保存したデータはSQLやPython分析スクリプトで自由に分析できる。

## 技術スタック

- **言語**: Go 1.25+（slack-go v0.20.0がGo 1.25を要求）
- **Slack SDK**: [slack-go/slack](https://github.com/slack-go/slack) v0.20.0
- **DB**: SQLite（[modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) — CGO不要の純Go実装）
- **CLI**: [cobra](https://github.com/spf13/cobra) + [viper](https://github.com/spf13/viper)
- **設定**: [gopkg.in/yaml.v3](https://pkg.go.dev/gopkg.in/yaml.v3)（channels.yml）
- **ログ**: [slog](https://pkg.go.dev/log/slog)（標準ライブラリ、JSON形式）
- **分析**: Python 3.12+（標準ライブラリのみ、外部依存なし）

## プロジェクト構造

```
slack_crawler/
├── CLAUDE.md                 # Claude Code向けガイダンス（このファイル）
├── README.md                 # クイックスタートガイド
├── channels.yml              # クロール対象チャンネル定義
├── .env.example              # 環境変数テンプレート
├── .env                      # 実際の環境変数（gitignore対象）
├── go.mod / go.sum
├── cmd/
│   └── slack-crawler/
│       └── main.go           # CLIエントリーポイント（cobra）
├── internal/
│   ├── config/               # 設定管理（.env + channels.yml + 環境変数）
│   ├── crawler/              # Slack APIクロール制御（インターフェース抽象化）
│   ├── store/                # SQLiteアクセス層（スキーマ自動マイグレーション）
│   └── model/                # データモデル（Channel, Message, User, CrawlLog）
├── scripts/
│   └── analyze.py            # メッセージ傾向分析（Stage1集計→Stage2圧縮→Markdown出力）
├── data/                     # SQLiteファイル格納（gitignore対象）
└── docs/
    ├── architecture.md       # アーキテクチャ概要・データフロー
    ├── implementation-plan.md # MVP計画・進捗管理
    ├── schema.md             # SQLiteスキーマ設計
    ├── setup.md              # Slack Appセットアップ手順
    └── slack-api.md          # Slack API利用方針・レートリミット対策
```

## モジュール設計

| パッケージ | 責務 |
|-----------|------|
| `cmd/slack-crawler` | CLIエントリーポイント。crawl/channels/stats/query/crawl-search サブコマンド |
| `internal/config` | `.env`・環境変数・`channels.yml` の読み込みと検証。User Token優先ロジック |
| `internal/crawler` | `SlackClient`インターフェースでAPI抽象化。ページネーション、レートリミット、スレッド差分検知 |
| `internal/store` | SQLite CRUD。UPSERT、スキーマ自動マイグレーション、crawl_logs、RawQuery |
| `internal/model` | Channel, Message, User, CrawlLog のデータ構造 |

## CLIサブコマンド

```bash
# チャンネルクロール（channels.yml または --channel で指定）
slack-crawler crawl [--channel <ID>] [--since YYYY-MM-DD] [--until YYYY-MM-DD]

# Slack検索APIでメッセージ収集（チャンネル横断）
slack-crawler crawl-search --user <USER_ID> [--since DATE] [--until DATE]
slack-crawler crawl-search --query "検索クエリ"

# 保存済みチャンネル一覧
slack-crawler channels

# チャンネル別メッセージ数
slack-crawler stats

# 任意SQL実行（テーブル形式出力）
slack-crawler query "SELECT ..."
```

## 設定

### トークン（2種類対応、User Token優先）

| 変数名 | 説明 |
|--------|------|
| `SLACK_USER_TOKEN` | User Token (`xoxp-`)。**推奨** — Bot招待不要 |
| `SLACK_BOT_TOKEN` | Bot Token (`xoxb-`)。チャンネルへのBot招待が必要 |

両方設定した場合は **User Tokenが優先** される。

### チャンネル指定（2つの方法）

1. **`channels.yml`**（推奨）: チャンネルID + 名前をYAML管理
2. **`SLACK_CHANNEL_IDS`**: カンマ区切りの環境変数（channels.ymlがなければフォールバック）

```yaml
# channels.yml の例
channels:
  - id: C07K0F15TQF
    name: guest-ito-shohei
```

### その他の環境変数

| 変数名 | 説明 | デフォルト |
|--------|------|-----------|
| `DB_PATH` | SQLiteファイルパス | `data/slack.db` |
| `LOG_LEVEL` | ログレベル（debug/info/warn/error） | `info` |

## 開発ガイドライン

### コーディング規約

- `go fmt` / `go vet` を常に通す
- `internal/` パッケージで外部公開を制御
- エラーは `fmt.Errorf("context: %w", err)` でラップ
- slogでの構造化ログ出力（JSON形式）
- テストは `*_test.go` でテーブル駆動テストを基本とする
- Slack APIは `SlackClient` インターフェースで抽象化し、テストではモック使用

### テスト実行

```bash
go vet ./...
go test ./... -count=1
```

現在25テスト（config 9, crawler 6, store 10）。全テストはSlack API不要（モック使用）。

### ビルド

```bash
go build -o slack-crawler ./cmd/slack-crawler
```

### Slack API利用方針

- User Token (`xoxp-`) を推奨（Bot招待不要でクロール可能）
- 必要なUser Token Scopes: `channels:history`, `channels:read`, `groups:history`, `groups:read`, `search:read`
- レートリミット: Tier 3（50 req/min → 1.2s間隔）、Tier 2（20 req/min → 3s間隔）
- 429レスポンス時は `Retry-After` に従う

### SQLiteテーブル

4テーブル構成（アプリ起動時に自動マイグレーション）:

- `channels` — チャンネルメタ情報
- `messages` — メッセージ本文（PK: ts + channel_id）。スレッド返信も同一テーブル
- `users` — ユーザー情報（クロール後に自動取得）
- `crawl_logs` — クロール実行履歴（開始/完了/失敗）

詳細は [docs/schema.md](docs/schema.md) 参照。

## 分析スクリプト

```bash
# ユーザーのメッセージ傾向分析（Markdown出力）
python3 scripts/analyze.py --user U07KF1QFLRY -o analysis.md
```

Stage 1（SQL集計: チャンネル別分布、時系列、メッセージ長）→ Stage 2（compaction: ノイズ除去、サンプリング、キーワード抽出）→ Markdown出力。出力をそのままLLMに投げて傾向分析に使える。

## 関連ドキュメント

- [セットアップガイド](docs/setup.md) — Slack Appの作成からトークン取得まで
- [アーキテクチャ概要](docs/architecture.md) — システム構成・データフロー
- [MVP計画](docs/implementation-plan.md) — 開発ロードマップ・進捗
- [SQLiteスキーマ設計](docs/schema.md) — テーブル定義・分析クエリ例
- [Slack API利用方針](docs/slack-api.md) — API仕様・レートリミット対策

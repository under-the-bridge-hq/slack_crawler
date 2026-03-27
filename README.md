# slack_crawler

Slackのメッセージを収集→SQLiteに保存→SQLで分析できるようにするCLIツール。

## 機能

- 指定チャンネルの全メッセージ（スレッド返信含む）をクロール
- SQLiteにローカル保存（WALモード）
- 差分クロール対応（前回以降の新着のみ取得）
- SQLで自由に分析可能

## セットアップ

```bash
# ビルド
go build -o slack-crawler ./cmd/slack-crawler

# 環境変数設定
cp .env.example .env
# .envを編集

# 実行
./slack-crawler crawl --channel C0XXXXXXX
```

## 環境変数

| 変数名 | 説明 |
|--------|------|
| `SLACK_BOT_TOKEN` | Slack Bot Token (`xoxb-`) |
| `SLACK_CHANNEL_IDS` | クロール対象チャンネルID（カンマ区切り） |
| `DB_PATH` | SQLiteファイルパス（デフォルト: `data/slack.db`） |

Slack Appのセットアップ手順は [docs/slack-api.md](docs/slack-api.md) を参照。

## ドキュメント

- [CLAUDE.md](CLAUDE.md) — プロジェクト詳細・開発ガイドライン
- [アーキテクチャ](docs/architecture.md) — システム構成・データフロー
- [MVP計画](docs/implementation-plan.md) — 開発ロードマップ
- [スキーマ設計](docs/schema.md) — SQLiteテーブル定義・分析クエリ例
- [Slack API](docs/slack-api.md) — API利用方針・レートリミット

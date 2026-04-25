# slack_crawler

Slackのメッセージを収集 → SQLiteに保存 → SQLや分析スクリプトで分析できるCLIツール。

## 機能

- 指定チャンネルの全メッセージ（スレッド返信含む）をクロール
- Slack検索APIによるチャンネル横断メッセージ収集
- SQLiteにローカル保存（WALモード、UPSERT）
- 差分クロール対応（前回以降の新着のみ取得）
- 期間指定（`--since` / `--until`）
- User Token / Bot Token 両対応（User Token推奨）
- メッセージ傾向分析スクリプト（Python）

## クイックスタート

```bash
# ビルド
go build -o slack-crawler ./cmd/slack-crawler

# 環境変数設定
cp .env.example .env
# .envを編集してSLACK_USER_TOKENを設定

# チャンネルクロール
./slack-crawler crawl --channel C0XXXXXXX

# 保存済みデータの確認
./slack-crawler stats
./slack-crawler channels
```

## サブコマンド

```bash
# チャンネルクロール（channels.yml の全チャンネルを一括処理）
./slack-crawler crawl

# 特定チャンネル + 期間指定
./slack-crawler crawl --channel C0XXXXXXX --since 2026-01-01 --until 2026-03-31

# Slack検索APIでユーザーの全発言を横断収集
./slack-crawler crawl-search --user U07KF1QFLRY --since 2026-01-01

# 任意SQL実行
./slack-crawler query "SELECT c.name, COUNT(*) FROM messages m JOIN channels c ON m.channel_id = c.id GROUP BY c.name ORDER BY 2 DESC"
```

## チャンネル設定

`channels.yml` でクロール対象を管理:

```yaml
channels:
  - id: C07K0F15TQF
    name: guest-ito-shohei
  - id: C0AN3M1UTBN
    name: ops-steward
```

環境変数 `SLACK_CHANNEL_IDS=C07K0F15TQF,C0AN3M1UTBN` でも指定可能（channels.yml が優先）。

## メッセージ分析

```bash
# ユーザーのメッセージ傾向をMarkdownで出力
python3 scripts/analyze.py --user U07KF1QFLRY -o analysis.md
```

## 環境変数

| 変数名 | 説明 | 必須 |
|--------|------|------|
| `SLACK_USER_TOKEN` | User Token (`xoxp-`)。推奨 | いずれか |
| `SLACK_BOT_TOKEN` | Bot Token (`xoxb-`) | いずれか |
| `SLACK_CHANNEL_IDS` | チャンネルID（カンマ区切り） | `channels.yml`がなければ |
| `DB_PATH` | SQLiteパス（デフォルト: `data/slack.db`） | |
| `LOG_LEVEL` | ログレベル（デフォルト: `info`） | |

## ドキュメント

- [セットアップガイド](docs/setup.md) — Slack Appの作成からトークン取得まで
- [CLAUDE.md](CLAUDE.md) — プロジェクト詳細・開発ガイドライン
- [アーキテクチャ](docs/architecture.md) — システム構成・データフロー
- [MVP計画](docs/implementation-plan.md) — 開発ロードマップ
- [スキーマ設計](docs/schema.md) — SQLiteテーブル定義・分析クエリ例
- [Slack API](docs/slack-api.md) — API利用方針・レートリミット

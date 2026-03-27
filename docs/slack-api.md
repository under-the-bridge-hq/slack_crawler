# Slack API利用方針

## 認証

- **User Token**（`xoxp-`）推奨 — Bot招待なしでチャンネルをクロール可能
- **Bot Token**（`xoxb-`）も利用可 — チャンネルへのBot招待が必要
- Slack App作成 → Token Scopesを付与 → ワークスペースにインストール

## 必要なBot Token Scopes

| スコープ | 用途 |
|---------|------|
| `channels:history` | パブリックチャンネルのメッセージ取得 |
| `channels:read` | パブリックチャンネルの情報取得 |
| `groups:history` | プライベートチャンネルのメッセージ取得 |
| `groups:read` | プライベートチャンネルの情報取得 |
| `users:read` | ユーザー情報取得 |

## 使用するAPI

### conversations.list

- **用途**: チャンネル一覧取得
- **Tier**: Tier 2 (20 req/min)
- **ページネーション**: cursor

### conversations.history

- **用途**: チャンネルのメッセージ取得
- **Tier**: Tier 3 (50 req/min)
- **ページネーション**: cursor
- **主要パラメータ**:
  - `channel`: チャンネルID
  - `oldest`: 取得開始タイムスタンプ（差分クロール用）
  - `limit`: 1リクエストあたりの取得件数（最大1000、デフォルト100）

### conversations.replies

- **用途**: スレッド返信の取得
- **Tier**: Tier 3 (50 req/min)
- **ページネーション**: cursor
- **主要パラメータ**:
  - `channel`: チャンネルID
  - `ts`: スレッド親メッセージのタイムスタンプ

### users.info

- **用途**: ユーザー情報取得
- **Tier**: Tier 4 (100 req/min)

## レートリミット制御

### 基本方針

- API呼び出し間に適切なスリープを挿入
- Tier 3: 1分あたり50リクエスト → **リクエスト間に最低1.2秒**のインターバル
- 429レスポンス時: `Retry-After` ヘッダーの秒数だけ待機してリトライ

### 実装方針

```
リクエスト送信
  ↓
200 OK → 次のリクエスト（インターバル後）
  ↓
429 Too Many Requests → Retry-After秒待機 → リトライ
  ↓
その他エラー → ログ出力 → リトライ（最大3回、指数バックオフ）
```

## Slack Appセットアップ手順

詳細は [セットアップガイド](setup.md) を参照。

**概要:**

1. https://api.slack.com/apps でApp作成
2. User Token Scopes（推奨）またはBot Token Scopesに上記スコープを追加
3. ワークスペースにインストールしてトークンをコピー
4. `.env` に `SLACK_USER_TOKEN` または `SLACK_BOT_TOKEN` を設定
5. Bot Token使用時のみ、クロール対象チャンネルにBotを招待（`/invite @slack-crawler`）

# セットアップガイド

## 前提条件

- Go 1.23以上がインストール済み
- Slackワークスペースの管理者権限（またはApp作成権限）

## 1. Slack Appの作成

1. [Slack API: Applications](https://api.slack.com/apps) にアクセス
2. **Create New App** → **From scratch** を選択
3. App名（例: `slack-crawler`）を入力し、対象ワークスペースを選択
4. **Create App** をクリック

## 2. トークンの種類を選択

slack-crawlerは2種類のトークンに対応している。用途に応じて選択すること。

| トークン | 形式 | 特徴 |
|---------|------|------|
| **User Token（推奨）** | `xoxp-` | 自分がアクセス可能なチャンネルをBotの招待なしでクロール可能 |
| Bot Token | `xoxb-` | チャンネルへのBot招待が必須。組織管理用途向き |

> **推奨**: 個人の分析用途にはUser Tokenが便利。Bot招待なしでパブリックチャンネルをクロールでき、自分がメンバーのプライベートチャンネルにもアクセスできる。

## 3. スコープの設定

左メニューから **OAuth & Permissions** を開く。

### User Tokenを使う場合（推奨）

**User Token Scopes** に以下を追加する。

| スコープ | 用途 |
|---------|------|
| `channels:history` | パブリックチャンネルのメッセージ取得 |
| `channels:read` | パブリックチャンネルの情報取得 |
| `groups:history` | プライベートチャンネルのメッセージ取得 |
| `groups:read` | プライベートチャンネルの情報取得 |
| `search:read` | メッセージ検索（crawl-searchコマンド用） |

### Bot Tokenを使う場合

**Bot Token Scopes** に以下を追加する。

| スコープ | 用途 |
|---------|------|
| `channels:history` | パブリックチャンネルのメッセージ取得 |
| `channels:read` | パブリックチャンネルの情報取得 |
| `groups:history` | プライベートチャンネルのメッセージ取得 |
| `groups:read` | プライベートチャンネルの情報取得 |
| `users:read` | ユーザー情報取得 |

> **Note**: プライベートチャンネルのクロールが不要であれば `groups:history` と `groups:read` は省略可。

## 4. ワークスペースへのインストール

1. **OAuth & Permissions** ページ上部の **Install to Workspace** をクリック
2. 権限の確認画面で **許可する** を選択
3. トークンをコピー:
   - User Token: **User OAuth Token**（`xoxp-` で始まる文字列）
   - Bot Token: **Bot User OAuth Token**（`xoxb-` で始まる文字列）

## 5. 環境変数の設定

```bash
cp .env.example .env
```

`.env` を編集して以下を設定する。

```bash
# User Token（推奨）
SLACK_USER_TOKEN=xoxp-xxxx-xxxx-xxxx-xxxx

# または Bot Token
# SLACK_BOT_TOKEN=xoxb-xxxx-xxxx-xxxx

# クロール対象チャンネルID（カンマ区切りで複数指定可）
SLACK_CHANNEL_IDS=C0XXXXXXX,C0YYYYYYY
```

両方設定した場合は **User Tokenが優先** される。

> **チャンネルIDの確認方法**: Slackでチャンネルを開き、チャンネル名をクリック → モーダル最下部にチャンネルIDが表示される。

## 6. Botをチャンネルに招待（Bot Tokenの場合のみ）

Bot Tokenを使う場合は、クロール対象の各チャンネルで以下のコマンドを実行する。

```
/invite @slack-crawler
```

Botがチャンネルに参加していないと `not_in_channel` エラーになる。

> **User Tokenの場合はこの手順は不要。** 自分がアクセス可能なチャンネルを直接クロールできる。

## 7. ビルドと実行

```bash
# ビルド
go build -o slack-crawler ./cmd/slack-crawler

# チャンネル一覧の確認（クロール済みのもの）
./slack-crawler channels

# クロール実行
./slack-crawler crawl --channel C0XXXXXXX

# 統計表示
./slack-crawler stats
```

## トラブルシューティング

### `invalid_auth` エラー

- トークンが正しくコピーされているか確認
- User Token は `xoxp-`、Bot Token は `xoxb-` で始まる

### `not_in_channel` エラー

- **Bot Token使用時**: 対象チャンネルでBotを `/invite` しているか確認
- **User Token使用時**: 自分がそのチャンネルのメンバーか確認（プライベートチャンネルの場合）

### `missing_scope` エラー

- Slack App設定で必要なスコープが全て追加されているか確認
- スコープ追加後はAppの再インストールが必要（OAuth & Permissions → Reinstall to Workspace）

### `channel_not_found` エラー

- チャンネルIDが正しいか確認
- Bot Token使用時、Botがチャンネルに招待されていない可能性がある

# アーキテクチャ概要

## システム構成

```
┌──────────────┐     ┌──────────────────┐     ┌──────────────┐
│  Slack API   │◄───►│  slack-crawler    │────►│  SQLite      │
│  (Web API)   │     │  (Go CLI)        │     │  (data/)     │
└──────────────┘     └──────────────────┘     └──────────────┘
                            │
                     ┌──────┴──────┐
                     │  .env /     │
                     │  config     │
                     └─────────────┘
```

## データフロー

### クロール処理

```
1. CLI起動 → config読み込み → Bot Token検証
2. 対象チャンネルIDを取得
3. チャンネルごとにループ:
   a. conversations.history でメッセージ取得（cursorページネーション）
   b. スレッドがあるメッセージは conversations.replies で返信取得
   c. 取得したメッセージをSQLiteにUPSERT
   d. レートリミット制御（Tier 3: 50 req/min）
4. 完了サマリー出力
```

### 差分クロール

```
1. 前回クロール時の最新タイムスタンプをDBから取得
2. oldest パラメータに設定して差分のみ取得
3. 新規メッセージのみINSERT、更新メッセージはUPDATE
```

## モジュール構成

```
cmd/slack-crawler/main.go
  └─ cobra CLI定義
      ├─ crawl    → internal/crawler → internal/store
      ├─ channels → internal/crawler（チャンネル一覧取得）
      ├─ stats    → internal/store（統計情報出力）
      └─ query    → internal/store（任意SQL実行）

internal/
├── config/    設定管理（viper: env, flag, config file）
├── crawler/   Slack API呼び出し・ページネーション・レートリミット
├── store/     SQLite CRUD・スキーマ管理・マイグレーション
└── model/     Channel, Message, User等のデータ構造
```

## 技術選定理由

| 選択 | 理由 |
|------|------|
| Go | CLIツール。シングルバイナリ配布。複雑な非同期処理不要 |
| modernc.org/sqlite | CGO不要の純Go SQLite実装。クロスコンパイル容易 |
| cobra + viper | Goの標準的なCLIフレームワーク。サブコマンド・設定管理の統合 |
| slog | Go 1.21+標準。構造化ログ。外部依存なし |
| slack-go/slack | 成熟したSlack Go SDK。Web API完全サポート |

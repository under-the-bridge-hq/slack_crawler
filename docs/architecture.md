# アーキテクチャ概要

## システム構成

```
┌──────────────┐     ┌──────────────────┐     ┌──────────────┐
│  Slack API   │◄───►│  slack-crawler    │────►│  SQLite      │
│  (Web API)   │     │  (Go CLI)        │     │  (data/)     │
└──────────────┘     └──────────────────┘     └──────────────┘
                            │                        │
                     ┌──────┴──────┐          ┌──────┴──────┐
                     │  .env /     │          │  analyze.py │
                     │  channels.  │          │  (Python)   │
                     │  yml        │          └─────────────┘
                     └─────────────┘
```

## データフロー

### crawl コマンド

```
1. CLI起動 → config読み込み（.env + channels.yml）→ トークン検証
2. 対象チャンネルIDを取得（channels.yml or SLACK_CHANNEL_IDS or --channel）
3. チャンネルごとにループ:
   a. conversations.info でチャンネル情報取得 → channels テーブルにUPSERT
   b. crawl_logs に開始記録
   c. conversations.history でメッセージ取得（cursorページネーション）
      - 差分クロール: 前回の最新ts以降のみ取得
      - --since指定時はそちらを優先
   d. スレッド差分検知: reply_countが変わったスレッドのみ
      conversations.replies で返信取得
   e. crawl_logs に完了記録
4. 未登録ユーザーの情報を users.info で自動取得
5. 完了サマリー出力
```

### crawl-search コマンド

```
1. 検索クエリ構築（--user / --query / --since / --until）
2. search.messages API でページネーション取得（Tier 2: 3s間隔）
3. 結果からチャンネル情報・メッセージをそれぞれUPSERT
```

### 差分クロール

```
1. 前回クロール時の最新タイムスタンプをDBから取得
2. oldest パラメータに設定して差分のみ取得
3. 新規メッセージのみINSERT、更新メッセージはUPDATE（UPSERT）
4. スレッド: reply_countが変わったスレッドのみ再取得（全スレッド再取得を回避）
```

## モジュール構成

```
cmd/slack-crawler/main.go
  └─ cobra CLI定義
      ├─ crawl        → internal/crawler → internal/store
      ├─ crawl-search → internal/crawler（検索API）→ internal/store
      ├─ channels     → internal/store（保存済みチャンネル一覧）
      ├─ stats        → internal/store（統計情報出力）
      └─ query        → internal/store（任意SQL実行、テーブル形式出力）

internal/
├── config/    設定管理（viper: .env + 環境変数 / yaml: channels.yml）
│              User Token優先ロジック
├── crawler/   SlackClientインターフェース → 実クライアント / モック
│              ページネーション・レートリミット・スレッド差分・ユーザー取得
├── store/     SQLite CRUD・UPSERT・スキーマ自動マイグレーション
│              crawl_logs・RawQuery
└── model/     Channel, Message, User, CrawlLog

scripts/
└── analyze.py  SQLiteから直接読み込み → 集計 → compaction → Markdown出力
```

## テスト戦略

- `internal/crawler` の `SlackClient` インターフェースにより、テストは全てモックで実行
- Slack APIトークン不要で `go test ./...` が完結する
- `internal/store` はインメモリSQLite（tmpdir）でテスト

## 技術選定理由

| 選択 | 理由 |
|------|------|
| Go | CLIツール。シングルバイナリ配布。複雑な非同期処理不要 |
| modernc.org/sqlite | CGO不要の純Go SQLite実装。クロスコンパイル容易 |
| cobra + viper | Goの標準的なCLIフレームワーク。サブコマンド・設定管理の統合 |
| slog | Go 1.21+標準。構造化ログ。外部依存なし |
| slack-go/slack | 成熟したSlack Go SDK。Web API完全サポート |
| Python (分析) | 形態素解析・統計処理のエコシステムが充実。標準ライブラリのみで初期実装 |

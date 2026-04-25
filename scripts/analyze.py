#!/usr/bin/env python3
"""
Slackメッセージ傾向分析スクリプト

SQLiteからデータを読み込み、Stage 1（SQL集計）→ Stage 2（compaction）→ Markdown出力。
出力されたMarkdownはそのままLLMに投げて傾向分析に使える。

Usage:
    python3 scripts/analyze.py --user U07KF1QFLRY
    python3 scripts/analyze.py --user U07KF1QFLRY --db data/slack.db
    python3 scripts/analyze.py --user U07KF1QFLRY --sample 15 --min-length 20
"""

import argparse
import random
import re
import sqlite3
import sys
from collections import Counter
from pathlib import Path


def connect(db_path: str) -> sqlite3.Connection:
    conn = sqlite3.connect(db_path)
    conn.row_factory = sqlite3.Row
    return conn


# ---------------------------------------------------------------------------
# Stage 1: SQL集計
# ---------------------------------------------------------------------------

def stage1_stats(conn: sqlite3.Connection, user_id: str) -> dict:
    stats = {}

    # 基本統計
    row = conn.execute("""
        SELECT COUNT(*) as cnt,
               SUM(LENGTH(text)) as total_chars,
               AVG(LENGTH(text)) as avg_chars,
               MIN(created_at) as first_msg,
               MAX(created_at) as last_msg
        FROM messages WHERE user_id = ?
    """, (user_id,)).fetchone()
    stats["total_messages"] = row["cnt"]
    stats["total_chars"] = row["total_chars"] or 0
    stats["avg_chars"] = round(row["avg_chars"] or 0, 1)
    stats["first_message"] = row["first_msg"]
    stats["last_message"] = row["last_msg"]

    # ユーザー名
    user = conn.execute(
        "SELECT name, real_name, display_name FROM users WHERE id = ?", (user_id,)
    ).fetchone()
    if user:
        stats["user_name"] = user["real_name"] or user["display_name"] or user["name"]
    else:
        stats["user_name"] = user_id

    # チャンネル別分布
    stats["channel_dist"] = conn.execute("""
        SELECT c.name, COUNT(*) as cnt, SUM(LENGTH(m.text)) as chars
        FROM messages m LEFT JOIN channels c ON m.channel_id = c.id
        WHERE m.user_id = ?
        GROUP BY c.name ORDER BY cnt DESC
    """, (user_id,)).fetchall()

    # 曜日別分布（0=日曜, 6=土曜 → 月曜始まりに変換）
    stats["weekday_dist"] = conn.execute("""
        SELECT CASE CAST(strftime('%w', created_at) AS INTEGER)
            WHEN 0 THEN '日' WHEN 1 THEN '月' WHEN 2 THEN '火'
            WHEN 3 THEN '水' WHEN 4 THEN '木' WHEN 5 THEN '金' WHEN 6 THEN '土'
        END as weekday,
        CAST(strftime('%w', created_at) AS INTEGER) as day_num,
        COUNT(*) as cnt
        FROM messages WHERE user_id = ?
        GROUP BY day_num ORDER BY day_num
    """, (user_id,)).fetchall()

    # 時間帯別分布
    stats["hour_dist"] = conn.execute("""
        SELECT CAST(strftime('%H', created_at, '+9 hours') AS INTEGER) as hour, COUNT(*) as cnt
        FROM messages WHERE user_id = ?
        GROUP BY hour ORDER BY hour
    """, (user_id,)).fetchall()

    # メッセージ長の分布
    stats["length_dist"] = conn.execute("""
        SELECT
            CASE
                WHEN LENGTH(text) <= 10 THEN '1-10'
                WHEN LENGTH(text) <= 30 THEN '11-30'
                WHEN LENGTH(text) <= 100 THEN '31-100'
                WHEN LENGTH(text) <= 300 THEN '101-300'
                ELSE '301+'
            END as bucket,
            COUNT(*) as cnt
        FROM messages WHERE user_id = ?
        GROUP BY bucket ORDER BY MIN(LENGTH(text))
    """, (user_id,)).fetchall()

    # スレッド参加率
    row = conn.execute("""
        SELECT
            COUNT(CASE WHEN thread_ts != '' AND thread_ts != ts THEN 1 END) as replies,
            COUNT(CASE WHEN reply_count > 0 THEN 1 END) as thread_starts
        FROM messages WHERE user_id = ?
    """, (user_id,)).fetchone()
    stats["thread_replies"] = row["replies"]
    stats["thread_starts"] = row["thread_starts"]

    # 月別推移
    stats["monthly"] = conn.execute("""
        SELECT strftime('%Y-%m', created_at) as month, COUNT(*) as cnt
        FROM messages WHERE user_id = ?
        GROUP BY month ORDER BY month
    """, (user_id,)).fetchall()

    return stats


# ---------------------------------------------------------------------------
# Stage 2: Compaction
# ---------------------------------------------------------------------------

NOISE_PATTERNS = re.compile(
    r"^(了解|ありがとう|おはよう|お疲れ|OK|ok|おk|はい|うん|なるほど|承知|ですね|👍|🙏|✅|⭕)"
)


def is_noise(text: str) -> bool:
    text = text.strip()
    if len(text) <= 5:
        return True
    if NOISE_PATTERNS.match(text):
        return True
    return False


def extract_keywords(texts: list[str], top_n: int = 30) -> list[tuple[str, int]]:
    """簡易キーワード抽出（形態素解析なし、正規表現ベース）"""
    # 日本語: 2文字以上のカタカナ語、英数字の単語を抽出
    word_counter: Counter = Counter()
    for text in texts:
        # カタカナ語（2文字以上）
        for m in re.finditer(r"[ァ-ヶー]{2,}", text):
            word_counter[m.group()] += 1
        # 英単語（3文字以上、URLやメンション除く）
        for m in re.finditer(r"\b[a-zA-Z][a-zA-Z0-9_-]{2,}\b", text):
            w = m.group().lower()
            if w not in {"https", "http", "the", "and", "for", "this", "that", "with", "from", "have", "been"}:
                word_counter[w] += 1
    return word_counter.most_common(top_n)


def stage2_compact(conn: sqlite3.Connection, user_id: str,
                   sample_per_channel: int = 10,
                   min_length: int = 20) -> dict:
    result = {}

    # 全メッセージ取得
    rows = conn.execute("""
        SELECT m.text, m.created_at, c.name as channel_name, LENGTH(m.text) as text_len
        FROM messages m LEFT JOIN channels c ON m.channel_id = c.id
        WHERE m.user_id = ? AND m.text != ''
        ORDER BY m.ts
    """, (user_id,)).fetchall()

    all_texts = [r["text"] for r in rows]

    # ノイズ除去
    meaningful = [r for r in rows if not is_noise(r["text"]) and r["text_len"] >= min_length]
    result["noise_removed"] = len(rows) - len(meaningful)
    result["meaningful_count"] = len(meaningful)

    # キーワード抽出
    result["keywords"] = extract_keywords(all_texts)

    # URL共有頻度
    url_count = sum(1 for t in all_texts if re.search(r"https?://", t))
    result["url_share_count"] = url_count

    # 質問文の割合
    question_count = sum(1 for t in all_texts if "？" in t or "?" in t)
    result["question_count"] = question_count

    # チャンネル別サンプリング
    by_channel: dict[str, list] = {}
    for r in meaningful:
        ch = r["channel_name"] or "unknown"
        by_channel.setdefault(ch, []).append(r)

    sampled = []
    for ch_name, msgs in sorted(by_channel.items(), key=lambda x: -len(x[1])):
        # 長文上位 + ランダム
        sorted_by_len = sorted(msgs, key=lambda x: -x["text_len"])
        half = sample_per_channel // 2
        top_long = sorted_by_len[:half]
        rest = sorted_by_len[half:]
        random_pick = random.sample(rest, min(half, len(rest))) if rest else []
        channel_sample = top_long + random_pick
        # 時系列順にソート
        channel_sample.sort(key=lambda x: x["created_at"])
        sampled.append((ch_name, len(msgs), channel_sample))

    result["sampled_channels"] = sampled

    # 圧縮後の総文字数
    total_sampled_chars = sum(
        len(r["text"]) for _, _, msgs in sampled for r in msgs
    )
    result["sampled_chars"] = total_sampled_chars

    return result


# ---------------------------------------------------------------------------
# Markdown出力
# ---------------------------------------------------------------------------

def render_markdown(stats: dict, compact: dict, user_id: str) -> str:
    lines = []
    a = lines.append

    a(f"# Slackメッセージ傾向分析: {stats['user_name']}")
    a(f"\n対象ユーザー: `{user_id}`")
    a(f"分析期間: {stats['first_message'][:10]} 〜 {stats['last_message'][:10]}")
    a("")

    # --- Stage 1 ---
    a("## Stage 1: 構造的事実（SQL集計）")
    a("")
    a("### 基本統計")
    a("")
    a(f"- 総メッセージ数: **{stats['total_messages']}**")
    a(f"- 総文字数: **{stats['total_chars']:,}**")
    a(f"- 平均文字数/メッセージ: **{stats['avg_chars']}**")
    a(f"- スレッド開始: {stats['thread_starts']}回 / 返信: {stats['thread_replies']}回")
    a(f"- URL共有: {compact['url_share_count']}回")
    a(f"- 質問文（？含む）: {compact['question_count']}回 ({compact['question_count']*100//max(stats['total_messages'],1)}%)")
    a("")

    a("### チャンネル別分布")
    a("")
    a("| チャンネル | 投稿数 | 文字数 |")
    a("|-----------|--------|--------|")
    for r in stats["channel_dist"]:
        a(f"| {r['name'] or 'DM'} | {r['cnt']} | {r['chars'] or 0:,} |")
    a("")

    a("### 月別推移")
    a("")
    a("| 月 | 投稿数 |")
    a("|-----|--------|")
    for r in stats["monthly"]:
        a(f"| {r['month']} | {r['cnt']} |")
    a("")

    a("### 曜日別分布")
    a("")
    a("| 曜日 | 投稿数 |")
    a("|------|--------|")
    for r in stats["weekday_dist"]:
        a(f"| {r['weekday']} | {r['cnt']} |")
    a("")

    a("### 時間帯別分布（JST）")
    a("")
    a("| 時間 | 投稿数 |")
    a("|------|--------|")
    for r in stats["hour_dist"]:
        a(f"| {r['hour']:02d}時 | {r['cnt']} |")
    a("")

    a("### メッセージ長分布")
    a("")
    a("| 文字数 | 件数 |")
    a("|--------|------|")
    for r in stats["length_dist"]:
        a(f"| {r['bucket']} | {r['cnt']} |")
    a("")

    # --- Stage 2 ---
    a("## Stage 2: Compaction（圧縮データ）")
    a("")
    a(f"- 有意メッセージ数: {compact['meaningful_count']}（ノイズ除去: {compact['noise_removed']}件）")
    a(f"- サンプリング後の総文字数: **{compact['sampled_chars']:,}**（推定 {compact['sampled_chars']*15//10:,} トークン）")
    a("")

    if compact["keywords"]:
        a("### 頻出キーワード")
        a("")
        kw_str = " / ".join(f"{w}({c})" for w, c in compact["keywords"][:20])
        a(kw_str)
        a("")

    a("### チャンネル別代表メッセージ")
    a("")
    for ch_name, total_count, msgs in compact["sampled_channels"]:
        a(f"#### {ch_name}（{total_count}件中{len(msgs)}件抽出）")
        a("")
        for r in msgs:
            date = r["created_at"][:10]
            text = r["text"].replace("\n", " ").strip()
            if len(text) > 300:
                text = text[:297] + "..."
            a(f"- [{date}] {text}")
        a("")

    a("---")
    a("")
    a("*このデータをLLMに投入して傾向分析を行ってください。*")
    a("*プロンプト例: 「上記のSlackメッセージデータから、このユーザーのコミュニケーションスタイル、関心領域、行動パターンを分析してください。」*")

    return "\n".join(lines)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(description="Slackメッセージ傾向分析")
    parser.add_argument("--user", required=True, help="対象ユーザーID (例: U07KF1QFLRY)")
    parser.add_argument("--db", default="data/slack.db", help="SQLiteファイルパス")
    parser.add_argument("--sample", type=int, default=10, help="チャンネルあたりのサンプル数")
    parser.add_argument("--min-length", type=int, default=20, help="有意メッセージの最低文字数")
    parser.add_argument("-o", "--output", help="出力ファイル（省略時はstdout）")
    args = parser.parse_args()

    db_path = Path(args.db)
    if not db_path.exists():
        print(f"エラー: {db_path} が見つかりません", file=sys.stderr)
        sys.exit(1)

    conn = connect(str(db_path))
    try:
        # データ存在確認
        cnt = conn.execute(
            "SELECT COUNT(*) FROM messages WHERE user_id = ?", (args.user,)
        ).fetchone()[0]
        if cnt == 0:
            print(f"エラー: ユーザー {args.user} のメッセージが見つかりません", file=sys.stderr)
            sys.exit(1)

        print(f"Stage 1: SQL集計中... ({cnt}メッセージ)", file=sys.stderr)
        stats = stage1_stats(conn, args.user)

        print(f"Stage 2: Compaction中...", file=sys.stderr)
        compact = stage2_compact(conn, args.user,
                                 sample_per_channel=args.sample,
                                 min_length=args.min_length)

        md = render_markdown(stats, compact, args.user)

        if args.output:
            Path(args.output).write_text(md, encoding="utf-8")
            print(f"出力: {args.output}", file=sys.stderr)
        else:
            print(md)

    finally:
        conn.close()


if __name__ == "__main__":
    main()

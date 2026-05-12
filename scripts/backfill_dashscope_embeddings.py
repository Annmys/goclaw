#!/usr/bin/env python3
import json
import os
import sys
import urllib.request

import psycopg2


DASHSCOPE_BATCH = 10


def embed_texts(api_key: str, texts: list[str]) -> list[list[float]]:
    if not texts:
        return []
    req = urllib.request.Request(
        "https://dashscope.aliyuncs.com/compatible-mode/v1/embeddings",
        data=json.dumps(
            {
                "model": "text-embedding-v4",
                "input": texts,
                "dimensions": 1536,
            }
        ).encode("utf-8"),
        headers={
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
        },
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=60) as resp:
        body = json.loads(resp.read().decode("utf-8"))
        return [item["embedding"] for item in body.get("data", [])]


def vector_literal(values: list[float]) -> str:
    return "[" + ",".join(format(v, ".10g") for v in values) + "]"


def backfill_agents(cur, api_key: str) -> int:
    cur.execute(
        """
        select id, coalesce(display_name, ''), coalesce(frontmatter, '')
        from agents
        where deleted_at is null
          and frontmatter is not null
          and frontmatter <> ''
          and embedding is null
        order by agent_key
        """
    )
    rows = cur.fetchall()
    updated = 0
    for row in rows:
        agent_id, display_name, frontmatter = row
        text = f"{display_name}: {frontmatter}" if frontmatter else display_name
        embeddings = embed_texts(api_key, [text])
        if not embeddings:
            continue
        cur.execute(
            "update agents set embedding = %s::vector where id = %s",
            (vector_literal(embeddings[0]), agent_id),
        )
        updated += 1
    return updated


def backfill_vault(cur, api_key: str) -> int:
    cur.execute(
        """
        select id, title, path, summary
        from vault_documents
        where summary is not null
          and summary <> ''
          and embedding is null
        order by created_at asc
        """
    )
    rows = cur.fetchall()
    updated = 0
    for start in range(0, len(rows), DASHSCOPE_BATCH):
        batch = rows[start : start + DASHSCOPE_BATCH]
        texts = [f"{title} {path} {summary}".strip() for _, title, path, summary in batch]
        embeddings = embed_texts(api_key, texts)
        for (doc_id, _, _, _), emb in zip(batch, embeddings):
            cur.execute(
                "update vault_documents set embedding = %s::vector where id = %s",
                (vector_literal(emb), doc_id),
            )
            updated += 1
    return updated


def main() -> int:
    api_key = os.environ.get("DASHSCOPE_API_KEY")
    dsn = os.environ.get("GOCLAW_POSTGRES_DSN")
    if not api_key:
        print("missing DASHSCOPE_API_KEY", file=sys.stderr)
        return 1
    if not dsn:
        print("missing GOCLAW_POSTGRES_DSN", file=sys.stderr)
        return 1

    conn = psycopg2.connect(dsn)
    conn.autocommit = False
    try:
        with conn.cursor() as cur:
            agents_updated = backfill_agents(cur, api_key)
            vault_updated = backfill_vault(cur, api_key)
        conn.commit()
    except Exception:
        conn.rollback()
        raise
    finally:
        conn.close()

    print(json.dumps({"agents_updated": agents_updated, "vault_updated": vault_updated}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

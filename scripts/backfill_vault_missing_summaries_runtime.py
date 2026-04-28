import json
import subprocess
import urllib.request

API_KEY = "sk-940532db5cb541abb526c91c2ccf221e"
DSN = "postgres://goclaw:aac491be1c5dd14b364f97a369abd12c@postgres:5432/goclaw?sslmode=disable"


def q(sql: str):
    out = subprocess.check_output(["psql", DSN, "-At", "-F", "\t", "-c", sql], text=True)
    return [line.split("\t") for line in out.strip().splitlines() if line.strip()]


def embed(texts):
    req = urllib.request.Request(
        "https://dashscope.aliyuncs.com/compatible-mode/v1/embeddings",
        data=json.dumps(
            {
                "model": "text-embedding-v4",
                "input": texts,
                "dimensions": 1536,
            }
        ).encode(),
        headers={
            "Authorization": f"Bearer {API_KEY}",
            "Content-Type": "application/json",
        },
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=60) as resp:
        body = json.loads(resp.read().decode())
        return [item["embedding"] for item in body["data"]]


def vec(values):
    return "[" + ",".join(format(v, ".10g") for v in values) + "]"


def synthesize_summary(path: str) -> str:
    norm = path.replace("\\", "/")
    parts = [p for p in norm.split("/") if p]
    base = parts[-1] if parts else norm
    parent = "/".join(parts[-3:-1]) if len(parts) >= 3 else "." if len(parts) <= 1 else "/".join(parts[:-1])
    return f"Document file {base} (from {parent})"


def main():
    rows = q(
        """
        select id::text, coalesce(title,''), coalesce(path,'')
        from vault_documents
        where (summary is null or summary = '')
        order by created_at asc
        """
    )
    updated = 0
    for i in range(0, len(rows), 10):
        batch = rows[i : i + 10]
        summaries = [synthesize_summary(path) for _, _, path in batch]
        texts = [f"{title} {path} {summary}".strip() for (_, title, path), summary in zip(batch, summaries)]
        embeddings = embed(texts)
        for (doc_id, _, _), summary, emb in zip(batch, summaries, embeddings):
            subprocess.check_call(
                [
                    "psql",
                    DSN,
                    "-c",
                    f"update vault_documents set summary = $$" + summary + f"$$, embedding = '{vec(emb)}'::vector where id = '{doc_id}'::uuid",
                ]
            )
            updated += 1
    print(json.dumps({"vault_missing_summary_updated": updated}, ensure_ascii=False))


if __name__ == "__main__":
    main()

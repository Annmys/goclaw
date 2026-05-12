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


def main():
    agents = q(
        """
        select id::text, coalesce(display_name,''), coalesce(frontmatter,'')
        from agents
        where deleted_at is null
          and frontmatter is not null
          and frontmatter <> ''
          and embedding is null
        order by agent_key
        """
    )
    agent_updates = 0
    for agent_id, display_name, frontmatter in agents:
        emb = embed([f"{display_name}: {frontmatter}"])[0]
        subprocess.check_call(
            [
                "psql",
                DSN,
                "-c",
                f"update agents set embedding = '{vec(emb)}'::vector where id = '{agent_id}'::uuid",
            ]
        )
        agent_updates += 1

    vault = q(
        """
        select id::text, coalesce(title,''), coalesce(path,''), coalesce(summary,'')
        from vault_documents
        where summary is not null
          and summary <> ''
          and embedding is null
        order by created_at asc
        """
    )
    vault_updates = 0
    for i in range(0, len(vault), 10):
        batch = vault[i : i + 10]
        texts = [f"{title} {path} {summary}".strip() for _, title, path, summary in batch]
        embs = embed(texts)
        for (doc_id, _, _, _), emb in zip(batch, embs):
            subprocess.check_call(
                [
                    "psql",
                    DSN,
                    "-c",
                    f"update vault_documents set embedding = '{vec(emb)}'::vector where id = '{doc_id}'::uuid",
                ]
            )
            vault_updates += 1

    print(json.dumps({"agents_updated": agent_updates, "vault_updated": vault_updates}, ensure_ascii=False))


if __name__ == "__main__":
    main()

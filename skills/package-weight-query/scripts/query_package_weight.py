#!/usr/bin/env python3
import json
import sqlite3
import sys
from pathlib import Path

ROOT_CANDIDATES = [
    Path("/mnt/source/product-package-weights"),
    Path("/mnt/target/product-package-weights"),
    Path(r"D:\数据\产品包装重量表"),
    Path("/mnt/target/flow-orders"),
]

SEARCH_COLUMNS = [
    "product_code",
    "material_code",
    "system_code",
    "product_model",
    "spec_model",
    "material_name",
    "material_part",
    "description",
    "query_key",
]

RETURN_COLUMNS = [
    "product_code",
    "material_code",
    "system_code",
    "product_model",
    "spec_model",
    "material_name",
    "material_part",
    "carton_size",
    "packaging_type",
    "packaging_form",
    "packing_qty",
    "weight_field",
    "raw_weight",
    "unit",
    "measure_basis",
    "weight_g_avg",
    "weight_g_min",
    "weight_g_max",
    "confidence",
    "source_sheet_name",
    "source_position",
    "description",
]


def is_valid_db(path: Path) -> bool:
    try:
        if not path.exists() or not path.is_file() or path.stat().st_size <= 0:
            return False
        conn = sqlite3.connect(path)
        try:
            cur = conn.cursor()
            cur.execute("select name from sqlite_master where type='table' and name='package_weights'")
            return cur.fetchone() is not None
        finally:
            conn.close()
    except Exception:
        return False


def find_weight_db() -> Path | None:
    candidates: list[Path] = []
    for root in ROOT_CANDIDATES:
        if root.exists():
            candidates.extend(root.glob("*.sqlite"))

    official = [p for p in candidates if p.name == "产品包装重量表.sqlite" and is_valid_db(p)]
    preferred = [p for p in candidates if "产品包装重量表" in p.name and p.name != "产品包装重量表-新测试.sqlite" and is_valid_db(p)]
    fallback = [p for p in candidates if p.name == "产品包装重量表-新测试.sqlite"]
    for path in [*official, *preferred, *fallback, *candidates]:
        if is_valid_db(path):
            return path
    return None


def query_weight(keyword: str, limit: int = 10) -> dict:
    db = find_weight_db()
    if db is None:
        return {
            "ok": False,
            "query": keyword,
            "error": "package weight sqlite not found",
            "searched_roots": [str(p) for p in ROOT_CANDIDATES],
        }

    like = f"%{keyword}%"
    where = " or ".join([f"coalesce({col}, '') like ?" for col in SEARCH_COLUMNS])
    params = [like] * len(SEARCH_COLUMNS)
    select_cols = ", ".join(RETURN_COLUMNS)
    sql = f"""
        select {select_cols}
        from package_weights
        where {where}
        order by
          case
            when product_code = ? then 0
            when material_code = ? then 1
            when system_code = ? then 2
            when material_name = ? then 3
            else 9
          end,
          confidence desc,
          id asc
        limit ?
    """
    params.extend([keyword, keyword, keyword, keyword, limit])

    conn = sqlite3.connect(db)
    try:
        conn.row_factory = sqlite3.Row
        cur = conn.cursor()
        cur.execute(sql, params)
        rows = [dict(row) for row in cur.fetchall()]
        return {
            "ok": bool(rows),
            "query": keyword,
            "weight_db": str(db),
            "count": len(rows),
            "results": rows,
            "error": None if rows else "no matching package weight records",
        }
    finally:
        conn.close()


def main() -> int:
    if len(sys.argv) < 2:
        print("Usage: query_package_weight.py <keyword> [limit]", file=sys.stderr)
        return 2

    keyword = sys.argv[1].strip()
    limit = int(sys.argv[2]) if len(sys.argv) >= 3 and sys.argv[2].isdigit() else 10
    if not keyword:
        print(json.dumps({"ok": False, "error": "empty keyword"}, ensure_ascii=False, indent=2))
        return 1

    result = query_weight(keyword, limit)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result.get("ok") else 1


if __name__ == "__main__":
    raise SystemExit(main())

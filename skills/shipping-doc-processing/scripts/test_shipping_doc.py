#!/usr/bin/env python3
import json
import sys
from pathlib import Path

from resolve_shipping_doc import resolve_shipping_doc


def main() -> int:
    if len(sys.argv) != 2:
        print("Usage: test_shipping_doc.py <shipping-file>", file=sys.stderr)
        return 2

    path = Path(sys.argv[1])
    if not path.exists():
        print(json.dumps({"error": f"file not found: {path}"}, ensure_ascii=False))
        return 1

    print(json.dumps(resolve_shipping_doc(path), ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

# GoClaw Local Knowledge Sync Plan

This plan captures the OpenHuman-style local-first knowledge sync approach for
GoClaw without changing existing business cron jobs yet.

## Goals

- Keep business knowledge local, inspectable, and tenant-aware.
- Sync Windows-visible folders into fast indexes and Markdown summaries.
- Preserve source paths, timestamps, hashes, and refresh status.
- Avoid replacing existing cron jobs until the new sync path passes regression.

## Source Registry

The first implementation should keep a source registry with:

- `id`: stable source id.
- `name`: user-facing source name.
- `path_windows`: Windows source path.
- `path_container`: container path when mounted.
- `tenant_scope`: `system`, `tenant`, or `team`.
- `sync_mode`: `scan`, `convert`, `watch`, or `manual`.
- `index_target`: SQLite, Markdown vault, or both.
- `last_sync_at`, `last_success_at`, `last_error`.
- `file_count`, `record_count`, `hash`.

## Initial Sources

| id | path | target | notes |
| --- | --- | --- | --- |
| `flow-orders` | `D:\数据\包装流转单` | SQLite + Markdown summary | Existing order mapping/content indexes stay source of truth until replaced. |
| `package-weights` | `D:\数据\产品包装重量表` | SQLite + Markdown summary | Must preserve new-format semantics from `产品包装重量表-新.xlsx`. |
| `packaging-data` | `D:\数据\包装资料` | SQLite + Markdown summary | Used by packaging calculation. |
| `label-templates` | `D:\数据\标签模板` | File manifest + preview metadata | Used by BarTender bridge. |
| `operation-log` | `D:\goclaw操作记录` | Markdown vault | Project continuity and rule extraction source. |

## Sync Output Contract

Each sync run should produce:

```json
{
  "status": "ok | blocked | error",
  "source_id": "package-weights",
  "started_at": "2026-05-20T00:00:00Z",
  "finished_at": "2026-05-20T00:00:10Z",
  "files_scanned": 1,
  "records_written": 1234,
  "outputs": [
    {
      "type": "sqlite",
      "path": "D:\\数据\\产品包装重量表\\产品包装重量表.sqlite"
    }
  ],
  "validation": {
    "passed": true,
    "checks": []
  },
  "warnings": []
}
```

## Safety Rules

- Existing production cron jobs remain active until the replacement sync job has
  passed repeated regression runs.
- Do not delete source files.
- Do not invent records when source rows are missing or malformed.
- Empty `.sqlite` files are invalid outputs and must be reported as `error`.
- Every sync must expose Windows-visible output paths.

## Implementation Phases

1. Add source registry and read-only status UI/API.
2. Add non-destructive scan mode that reports file counts and hashes.
3. Add per-source converters behind explicit flags.
4. Run converters in parallel with existing cron jobs and compare row counts.
5. Switch scheduled jobs only after validation passes.

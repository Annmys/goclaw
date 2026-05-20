# GoClaw Tool/CLI Catalog

This catalog is the registry plan for local software bridges and deterministic
business tools. It is intentionally separate from business skill content.

## Purpose

- Let agents discover reliable local tools before writing ad-hoc shell scripts.
- Standardize how tools expose JSON status, validation, preview, and rollback.
- Keep real software integrations auditable and recoverable.

## Catalog Entry Shape

Each tool should have a catalog entry with:

- `id`: stable lowercase identifier.
- `display_name`: user-facing name.
- `owner`: maintainer or responsible agent/team.
- `backend`: real backend used by the tool, for example BarTender, SQLite,
  openpyxl, or filesystem watcher.
- `command`: CLI command or internal tool name.
- `inputs`: accepted files, folders, or parameters.
- `outputs`: files, folders, JSON schema, preview artifacts.
- `validation`: checks run before reporting success.
- `safe_modes`: dry-run, preview-only, validate-only, or read-only modes.
- `risks`: destructive actions, external dependencies, permission requirements.
- `regression_fixtures`: paths to test data used for smoke/regression tests.

## Initial Target Catalog

| id | display_name | backend | purpose |
| --- | --- | --- | --- |
| `bartender-label-bridge` | BarTender 标签桥接 | BarTender 2022 | Generate real label previews and print-ready jobs from `.btw` templates. |
| `excel-format-validator` | Excel 格式验证器 | openpyxl | Verify merged cells, borders, column widths, images, formulas, and visible output paths. |
| `sqlite-business-index` | SQLite 业务索引查询 | SQLite | Query flow orders, package weights, packaging data, and index freshness. |
| `shared-folder-indexer` | 共享目录索引器 | filesystem + SQLite | Convert watched business folders into fast local indexes. |
| `skill-regression-runner` | Skill 回归测试执行器 | GoClaw evolution center | Run non-destructive smoke and regression checks before approval. |

## Required Behavior

- Tools should return JSON summaries even when they also produce files.
- Tools should keep user-visible outputs under Windows-accessible paths when the
  task creates user deliverables.
- Tools must not fake external backend output. If BarTender, Excel, or SQLite is
  unavailable, return `blocked` or `error`.
- Tools should support narrow queries to avoid large shell output.

## Relationship to Skills

Skills describe business workflows. Catalog tools provide deterministic actions
inside those workflows.

Example:

- `标签生成` skill decides label type, extracts order fields, asks missing
  questions, and calls `bartender-label-bridge`.
- `bartender-label-bridge` validates the template, creates preview artifacts,
  and returns JSON.

The skill owns business judgment; the catalog tool owns deterministic execution.

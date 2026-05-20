# GoClaw Skill/CLI Standard

This document defines the baseline contract for business skills and local CLI
bridges used by GoClaw agents.

## Goals

- Prefer deterministic local tools over ad-hoc shell exploration.
- Return structured machine-readable results that agents can verify.
- Preserve user-visible business outputs and never fake missing data.
- Make every important skill testable, auditable, and safely evolvable.

## Required Skill Metadata

Every business skill should keep these fields in `SKILL.md` frontmatter:

- `name`: user-facing name, Chinese is allowed.
- `slug`: stable execution identifier, lowercase English with hyphens.
- `description`: what the skill does and when to use it.
- `version`: integer version, incremented on every behavior change.
- `category`: business area, for example `shipping`, `label`, `packaging`.
- `inputs`: expected files, paths, or parameters.
- `outputs`: generated files and JSON result schema.
- `validation`: required self-checks before reporting success.

## CLI Contract

Local business CLIs should support:

- `--input <path>` or a clearly documented input directory.
- `--output <path>` or a clearly documented output directory.
- `--json` to return a single JSON object to stdout.
- `--dry-run` when the command can change files or external state.
- `--validate <path>` for regression checks when applicable.

The CLI must call the real backend when the backend is required for correctness.
For example, label previews must use the BarTender bridge when a `.btw` template
is required; a placeholder image is not a valid preview.

## JSON Result Schema

The top-level JSON result should follow this shape:

```json
{
  "status": "ok | blocked | error",
  "skill": "shipping-doc-processing",
  "version": 9,
  "order_numbers": ["XS26000000"],
  "data_sources": [
    {
      "type": "sqlite | excel | template | user_upload",
      "path": "D:\\data\\example.xlsx",
      "matched": true,
      "notes": "source-specific explanation"
    }
  ],
  "outputs": [
    {
      "type": "xlsx | png | pdf | json | folder",
      "path": "D:\\data\\storage\\...",
      "visible_to_user": true,
      "description": "completed shipping document"
    }
  ],
  "validation": {
    "passed": true,
    "checks": [
      {
        "name": "format",
        "passed": true,
        "details": "merged cells, borders, logo, column widths checked"
      }
    ]
  },
  "missing": [],
  "warnings": []
}
```

## Status Rules

- `ok`: output exists and validation passed.
- `blocked`: required data or user choice is missing; no fake fallback was used.
- `error`: tool failed unexpectedly and should include a clear error message.

Agents must not report success when `status` is not `ok`.

## Validation Rules

Business skills must verify their own output before sending it to the user.

- Shipping documents: order numbers, CI/EPL/PL sheet names, logo/images, merged
  cells, borders, column widths, Total region, package counts, weights, CBM,
  and multi-order completeness.
- Label generation: order fields, attachment fields, template selection, real
  preview output, print action metadata, and batch print sequence.
- Packaging calculation: source order rows, package database hits, C# logic
  parity notes, missing package data, and output folder path.
- Weight lookup: exact/normalized model key, source sheet/table, unit weight
  semantics, and missing matches.

## Evolution Rules

- Custom skills can be patched by the owning agent or approved admin flow.
- Core skills require review and approval before behavior changes.
- Every behavior change increments the skill version.
- Every regression fix adds or updates at least one test fixture.
- The old version remains recoverable from local backup and git history.

## Agent Usage Rules

- Use the skill/CLI first when the task matches a known business process.
- Use shell only for actual command execution or validation, not for simple file
  listing or repeated exploration.
- If a JSON result includes `missing` or `warnings`, surface them clearly.
- If output files are produced, provide the Windows-visible path.

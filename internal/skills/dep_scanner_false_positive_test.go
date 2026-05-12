package skills

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestScanFile_JSImportInsidePythonFString(t *testing.T) {
	dir := t.TempDir()
	pyFile := filepath.Join(dir, "render.py")

	// Python file with JS ES module import inside f-string (issue #544)
	content := `#!/usr/bin/env python3
import sys
import json

def render_html(text):
    mermaid_init = f"""
<script type="module">
    import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.esm.min.mjs';
    mermaid.initialize({{ startOnLoad: true }});
</script>
"""
    return f"<html>{text}{mermaid_init}</html>"
`
	if err := os.WriteFile(pyFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	pyImports := make(map[string]bool)
	nodeImports := make(map[string]bool)
	binaries := make(map[string]bool)

	scanFile(pyFile, pyImports, nodeImports, binaries)

	// sys and json are real Python imports — should be detected
	if !pyImports["sys"] {
		t.Error("expected sys to be detected as Python import")
	}
	if !pyImports["json"] {
		t.Error("expected json to be detected as Python import")
	}

	// mermaid is a JS import inside f-string — should NOT be detected
	if pyImports["mermaid"] {
		t.Error("FALSE POSITIVE: mermaid detected as Python import — it's a JS import inside f-string")
	}
}

func TestScanFile_MultipleJSImportsInsidePythonString(t *testing.T) {
	dir := t.TempDir()
	pyFile := filepath.Join(dir, "template.py")

	// Multiple JS imports inside a Python string + real Python imports
	content := `import os
import subprocess

TEMPLATE = """
<script type="module">
    import React from 'https://cdn.example.com/react.js';
    import lodash from 'https://cdn.example.com/lodash.js';
</script>
"""
`
	if err := os.WriteFile(pyFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	pyImports := make(map[string]bool)
	nodeImports := make(map[string]bool)
	binaries := make(map[string]bool)

	scanFile(pyFile, pyImports, nodeImports, binaries)

	// Real Python imports
	if !pyImports["os"] {
		t.Error("expected os to be detected as Python import")
	}
	if !pyImports["subprocess"] {
		t.Error("expected subprocess to be detected as Python import")
	}

	// JS imports inside string — should NOT be detected
	if pyImports["React"] {
		t.Error("FALSE POSITIVE: React detected as Python import")
	}
	if pyImports["lodash"] {
		t.Error("FALSE POSITIVE: lodash detected as Python import")
	}
}

func TestScanScriptsDir_FiltersStdlib(t *testing.T) {
	scriptsDir := filepath.Join(t.TempDir(), "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Script imports stdlib modules + one real pip dep
	content := `import sys
import os
import json
import argparse
import subprocess
from pathlib import Path
from datetime import datetime

import requests
from PIL import Image
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "main.py"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m := scanScriptsDir(scriptsDir)

	// Only real pip deps should appear in RequiresPython — NOT stdlib.
	for _, pkg := range m.RequiresPython {
		if pythonStdlib[pkg] {
			t.Errorf("stdlib module %q should have been filtered from RequiresPython", pkg)
		}
	}

	// Real deps must be present.
	if !slices.Contains(m.RequiresPython, "requests") {
		t.Error("expected 'requests' in RequiresPython")
	}
	if !slices.Contains(m.RequiresPython, "PIL") {
		t.Error("expected 'PIL' in RequiresPython")
	}

	// Stdlib must NOT be present.
	for _, stdlib := range []string{"sys", "os", "json", "argparse", "subprocess", "pathlib", "datetime"} {
		if slices.Contains(m.RequiresPython, stdlib) {
			t.Errorf("stdlib %q should NOT be in RequiresPython", stdlib)
		}
	}
}

func TestScanFile_SQLFromClauseInsidePythonStringDoesNotCreateFakeImport(t *testing.T) {
	dir := t.TempDir()
	pyFile := filepath.Join(dir, "query.py")

	content := `#!/usr/bin/env python3
import sqlite3

SQL = """
select workbook_path, sheet_name
from order_mapping
where order_no = ?
"""

SQL2 = """
select outer_box_spec
from flow_content_index
where order_no = ?
"""
`
	if err := os.WriteFile(pyFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	pyImports := make(map[string]bool)
	nodeImports := make(map[string]bool)
	binaries := make(map[string]bool)

	scanFile(pyFile, pyImports, nodeImports, binaries)

	if !pyImports["sqlite3"] {
		t.Error("expected sqlite3 to be detected as Python import")
	}
	if pyImports["order_mapping"] {
		t.Error("FALSE POSITIVE: order_mapping detected as Python import from SQL clause")
	}
	if pyImports["flow_content_index"] {
		t.Error("FALSE POSITIVE: flow_content_index detected as Python import from SQL clause")
	}
}

func TestScanSkillDeps_ManifestDepsOverrideScan(t *testing.T) {
	dir := t.TempDir()
	scriptsDir := filepath.Join(dir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatal(err)
	}

	skill := `---
name: test-skill
deps:
  - pip:psycopg2
  - npm:typescript
  - system:ffmpeg
---
# body
`
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skill), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "main.py"), []byte("import requests\n"), 0644); err != nil {
		t.Fatal(err)
	}

	m := ScanSkillDeps(dir)
	if !m.FromManifest {
		t.Fatal("expected manifest override to be active")
	}
	if !slices.Contains(m.RequiresPython, "psycopg2") {
		t.Fatalf("expected psycopg2 from manifest, got %#v", m.RequiresPython)
	}
	if slices.Contains(m.RequiresPython, "requests") {
		t.Fatalf("scan result should be overridden by manifest deps, got %#v", m.RequiresPython)
	}
	if !slices.Contains(m.RequiresNode, "typescript") {
		t.Fatalf("expected typescript from manifest, got %#v", m.RequiresNode)
	}
	if !slices.Contains(m.Requires, "ffmpeg") {
		t.Fatalf("expected ffmpeg from manifest, got %#v", m.Requires)
	}
}

func TestScanSkillDeps_ExcludeDepsFiltersScan(t *testing.T) {
	dir := t.TempDir()
	scriptsDir := filepath.Join(dir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatal(err)
	}

	skill := `---
name: test-skill
exclude_deps:
  - pip:requests
  - npm:lodash
  - ffmpeg
---
# body
`
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skill), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "main.py"), []byte("import requests\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "main.js"), []byte("require('lodash')\n"), 0644); err != nil {
		t.Fatal(err)
	}

	m := ScanSkillDeps(dir)
	if slices.Contains(m.RequiresPython, "requests") {
		t.Fatalf("expected requests to be excluded, got %#v", m.RequiresPython)
	}
	if slices.Contains(m.RequiresNode, "lodash") {
		t.Fatalf("expected lodash to be excluded, got %#v", m.RequiresNode)
	}
}

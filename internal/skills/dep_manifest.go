package skills

import (
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	pythonIdentRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.\-]*$`)
	npmPkgNameRe  = regexp.MustCompile(`^(@[a-z0-9][a-z0-9_.\-]*/)?[a-z0-9][a-z0-9_.\-]*$`)
	sysBinRe      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+\-]*$`)
)

type ParsedDep struct {
	Raw         string
	Category    string
	ImportName  string
	InstallSpec string
}

func isValidDepName(category, name string) bool {
	if name == "" {
		return false
	}
	switch category {
	case "pip":
		return pythonIdentRe.MatchString(name)
	case "npm":
		return npmPkgNameRe.MatchString(name)
	case "system":
		return sysBinRe.MatchString(name)
	case "github":
		return true
	}
	return false
}

func categorizeManifestDep(raw string) ParsedDep {
	p := ParsedDep{Raw: raw}
	switch {
	case strings.HasPrefix(raw, "pip:"):
		p.Category = "pip"
		spec := strings.TrimPrefix(raw, "pip:")
		p.ImportName, p.InstallSpec = splitPipSpec(spec)
	case strings.HasPrefix(raw, "npm:"):
		p.Category = "npm"
		p.ImportName = strings.TrimPrefix(raw, "npm:")
		p.InstallSpec = p.ImportName
	case strings.HasPrefix(raw, "github:"):
		p.Category = "github"
		p.InstallSpec = raw
	case strings.HasPrefix(raw, "system:"):
		p.Category = "system"
		p.ImportName = strings.TrimPrefix(raw, "system:")
		p.InstallSpec = p.ImportName
	default:
		p.Category = "system"
		p.ImportName = raw
		p.InstallSpec = raw
	}
	return p
}

func splitPipSpec(spec string) (importName, installSpec string) {
	installSpec = spec
	importName = spec
	for _, op := range []string{">=", "<=", "==", "!=", "~=", ">", "<"} {
		i := strings.Index(importName, op)
		if i == 0 {
			return "", installSpec
		}
		if i > 0 {
			importName = strings.TrimSpace(importName[:i])
			break
		}
	}
	if i := strings.IndexByte(importName, '['); i == 0 {
		return "", installSpec
	} else if i > 0 {
		importName = importName[:i]
	}
	return importName, installSpec
}

func skillMdPath(skillDir string) string {
	return filepath.Join(skillDir, "SKILL.md")
}

func parseSkillManifestFile(path string) (deps []string, excludeDeps []string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}
	fm := extractFrontmatter(string(data))
	if fm == "" {
		return nil, nil
	}
	lists := parseSimpleYAMLLists(fm)
	return lists["deps"], lists["exclude_deps"]
}

func applyManifestOverride(scan *SkillManifest, explicit, excludeDeps []string) *SkillManifest {
	if scan == nil {
		scan = &SkillManifest{}
	}
	scan.ExcludeDeps = excludeDeps

	if len(explicit) == 0 {
		if len(excludeDeps) > 0 {
			scan.RequiresPython = filterOutByImportName(scan.RequiresPython, excludeDeps, "pip")
			scan.RequiresNode = filterOutByImportName(scan.RequiresNode, excludeDeps, "npm")
			scan.Requires = filterOutByImportName(scan.Requires, excludeDeps, "system")
		}
		return scan
	}

	scan.FromManifest = true
	scan.Explicit = explicit
	var sysReq, pyReq, nodeReq []string
	for _, raw := range explicit {
		p := categorizeManifestDep(raw)
		if p.Category != "github" && !isValidDepName(p.Category, p.ImportName) {
			slog.Warn("skills: dropping invalid manifest dep",
				"raw", raw, "category", p.Category, "import_name", p.ImportName)
			continue
		}
		switch p.Category {
		case "pip":
			pyReq = append(pyReq, p.ImportName)
		case "npm":
			nodeReq = append(nodeReq, p.ImportName)
		case "system":
			sysReq = append(sysReq, p.ImportName)
		}
	}
	scan.Requires = sysReq
	scan.RequiresPython = pyReq
	scan.RequiresNode = nodeReq
	return scan
}

func filterOutByImportName(names, excludeDeps []string, category string) []string {
	if len(names) == 0 || len(excludeDeps) == 0 {
		return names
	}
	blocked := make(map[string]bool)
	prefix := category + ":"
	for _, e := range excludeDeps {
		switch {
		case strings.HasPrefix(e, prefix):
			name, _ := splitPipSpec(strings.TrimPrefix(e, prefix))
			if name != "" {
				blocked[name] = true
			}
		case category == "system" && !strings.Contains(e, ":"):
			blocked[e] = true
		}
	}
	if len(blocked) == 0 {
		return names
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if !blocked[n] {
			out = append(out, n)
		}
	}
	return out
}

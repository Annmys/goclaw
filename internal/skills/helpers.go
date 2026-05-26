package skills

import (
	"path/filepath"
	"regexp"
	"strings"
)

// SlugRegexp validates skill slugs: lowercase alphanumeric with hyphens, no leading/trailing hyphen.
var SlugRegexp = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)

// ParseSkillFrontmatter extracts name, description, and slug from SKILL.md YAML frontmatter.
// Also returns the full parsed frontmatter as a map for DB storage.
func ParseSkillFrontmatter(content string) (name, description, slug string, allFields map[string]string) {
	allFields = make(map[string]string)
	if !strings.HasPrefix(content, "---") {
		return "", "", "", allFields
	}
	end := strings.Index(content[3:], "---")
	if end < 0 {
		return "", "", "", allFields
	}
	fm := content[3 : 3+end]
	allFields = parseSimpleYAML(fm)
	name = allFields["name"]
	description = allFields["description"]
	slug = allFields["slug"]
	return
}

// GovernanceMetadata describes how a skill participates in a business skill family.
// Explicit frontmatter fields keep this tenant configurable without a schema migration.
type GovernanceMetadata struct {
	Name        string
	Description string
	Slug        string
	DisplayName string
	Family      string
	Canonical   bool
	Replaces    []string
	Aliases     []string
}

// ParseSkillGovernance extracts optional skill governance fields from SKILL.md frontmatter.
func ParseSkillGovernance(content string) GovernanceMetadata {
	_, _, _, fields := ParseSkillFrontmatter(content)
	return GovernanceFromFields(fields)
}

// GovernanceFromFields converts stored frontmatter fields into governance metadata.
func GovernanceFromFields(fields map[string]string) GovernanceMetadata {
	if fields == nil {
		fields = map[string]string{}
	}
	return GovernanceMetadata{
		Name:        fields["name"],
		Description: fields["description"],
		Slug:        fields["slug"],
		DisplayName: firstNonEmpty(fields["display_name"], fields["display-name"], fields["display"]),
		Family:      fields["family"],
		Canonical:   parseFrontmatterBool(fields["canonical"]),
		Replaces:    splitFrontmatterList(fields["replaces"]),
		Aliases:     splitFrontmatterList(fields["aliases"]),
	}
}

// InjectGovernanceFields writes normalized governance metadata back to frontmatter.
func InjectGovernanceFields(fields map[string]string, meta GovernanceMetadata) map[string]string {
	if fields == nil {
		fields = map[string]string{}
	}
	if meta.Name != "" {
		fields["name"] = meta.Name
	}
	if meta.Description != "" {
		fields["description"] = meta.Description
	}
	if meta.Slug != "" {
		fields["slug"] = meta.Slug
	}
	if meta.DisplayName != "" {
		fields["display_name"] = meta.DisplayName
	}
	if meta.Family != "" {
		fields["family"] = meta.Family
	}
	if meta.Canonical {
		fields["canonical"] = "true"
	}
	if len(meta.Replaces) > 0 {
		fields["replaces"] = strings.Join(meta.Replaces, ", ")
	}
	if len(meta.Aliases) > 0 {
		fields["aliases"] = strings.Join(meta.Aliases, ", ")
	}
	return fields
}

// FamilyKey returns the stable family key used to collapse same-domain skills.
func (g GovernanceMetadata) FamilyKey(fallbackSlug string) string {
	for _, v := range []string{g.Family, firstString(g.Replaces), g.Slug, g.DisplayName, g.Name, fallbackSlug} {
		if strings.TrimSpace(v) != "" {
			return Slugify(v)
		}
	}
	return "skill"
}

func parseFrontmatterBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func splitFrontmatterList(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	v = strings.Trim(v, "[]")
	parts := strings.FieldsFunc(v, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(strings.Trim(p, `"'`))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// Slugify converts a skill name into a valid slug (lowercase, alphanumeric + hyphens).
func Slugify(name string) string {
	s := strings.ToLower(name)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, s)
	// Collapse multiple dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	if s == "" {
		s = "skill"
	}
	return s
}

// IsSystemArtifact returns true for OS-generated junk that should be skipped
// during file extraction and listing (e.g. __MACOSX, .DS_Store, Thumbs.db).
func IsSystemArtifact(name string) bool {
	base := filepath.Base(name)
	// macOS resource fork / metadata folders and files
	if base == "__MACOSX" || strings.HasPrefix(base, "._") {
		return true
	}
	// Check if any path component is __MACOSX
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		if part == "__MACOSX" {
			return true
		}
	}
	// Common OS junk files
	switch base {
	case ".DS_Store", "Thumbs.db", "desktop.ini", ".Spotlight-V100", ".Trashes", ".fseventsd":
		return true
	}
	return false
}

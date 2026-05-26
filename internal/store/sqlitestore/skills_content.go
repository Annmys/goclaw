//go:build sqlite || sqliteonly

package sqlitestore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func (s *SQLiteSkillStore) LoadSkill(ctx context.Context, name string) (string, bool) {
	var slug string
	var version int
	var filePath *string
	q := `SELECT slug, version, file_path FROM skills
		WHERE (slug = ? OR lower(slug) = lower(?) OR name = ?) AND status = 'active'`
	args := []any{name, name, name}
	if !store.IsCrossTenant(ctx) {
		tid := store.TenantIDFromContext(ctx)
		if tid == uuid.Nil {
			tid = store.MasterTenantID
		}
		q += " AND (is_system = 1 OR tenant_id = ?)"
		args = append(args, tid)
	}
	q += " ORDER BY CASE WHEN slug = ? THEN 0 WHEN lower(slug) = lower(?) THEN 1 WHEN name = ? THEN 2 ELSE 3 END LIMIT 1"
	args = append(args, name, name, name)
	if err := s.db.QueryRowContext(ctx, q, args...).Scan(&slug, &version, &filePath); err != nil {
		return "", false
	}
	info := buildSkillInfo("", "", slug, nil, version, s.baseDir, filePath)
	content, err := readSkillFile(info.Path)
	if err != nil {
		return "", false
	}
	return content, true
}

func (s *SQLiteSkillStore) LoadForContext(ctx context.Context, allowList []string) string {
	skills := s.FilterSkills(ctx, allowList)
	if len(skills) == 0 {
		return ""
	}
	var parts []string
	for _, sk := range skills {
		content, ok := s.LoadSkill(ctx, sk.Slug)
		if !ok {
			continue
		}
		parts = append(parts, fmt.Sprintf("### Skill: %s（%s）V%d\n\n%s", sk.Slug, sk.Name, sk.Version, content))
	}
	if len(parts) == 0 {
		return ""
	}
	var result strings.Builder
	result.WriteString("## Available Skills\n\n")
	for i, p := range parts {
		if i > 0 {
			result.WriteString("\n\n---\n\n")
		}
		result.WriteString(p)
	}
	return result.String()
}

func (s *SQLiteSkillStore) BuildSummary(ctx context.Context, allowList []string) string {
	skills := s.FilterSkills(ctx, allowList)
	if len(skills) == 0 {
		return ""
	}
	var result strings.Builder
	result.WriteString("<available_skills>\n")
	for _, sk := range skills {
		result.WriteString("  <skill>\n")
		result.WriteString(fmt.Sprintf("    <slug>%s</slug>\n", sk.Slug))
		result.WriteString(fmt.Sprintf("    <name>%s</name>\n", sk.Name))
		displayName := sk.Name
		if sk.DisplayName != "" {
			displayName = sk.DisplayName
		}
		result.WriteString(fmt.Sprintf("    <display>%s（%s）V%d</display>\n", sk.Slug, displayName, sk.Version))
		result.WriteString(fmt.Sprintf("    <description>%s</description>\n", sk.Description))
		result.WriteString(fmt.Sprintf("    <location>%s</location>\n", sk.Path))
		if sk.Family != "" {
			result.WriteString(fmt.Sprintf("    <family>%s</family>\n", sk.Family))
		}
		if sk.Canonical {
			result.WriteString("    <canonical>true</canonical>\n")
		}
		result.WriteString("  </skill>\n")
	}
	result.WriteString("</available_skills>")
	return result.String()
}

func (s *SQLiteSkillStore) GetSkill(ctx context.Context, name string) (*store.SkillInfo, bool) {
	var id uuid.UUID
	var skillName, slug, visibility string
	var desc *string
	var tagsJSON, fmRaw []byte
	var version int
	var isSystem bool
	var filePath *string
	q := `SELECT id, name, slug, description, visibility, tags, version, is_system, frontmatter, file_path FROM skills
		WHERE (slug = ? OR lower(slug) = lower(?) OR name = ?) AND status = 'active'`
	args := []any{name, name, name}
	if !store.IsCrossTenant(ctx) {
		tid := store.TenantIDFromContext(ctx)
		if tid == uuid.Nil {
			tid = store.MasterTenantID
		}
		q += " AND (is_system = 1 OR tenant_id = ?)"
		args = append(args, tid)
	}
	q += " ORDER BY CASE WHEN slug = ? THEN 0 WHEN lower(slug) = lower(?) THEN 1 WHEN name = ? THEN 2 ELSE 3 END LIMIT 1"
	args = append(args, name, name, name)
	if err := s.db.QueryRowContext(ctx, q, args...).Scan(&id, &skillName, &slug, &desc, &visibility, &tagsJSON, &version, &isSystem, &fmRaw, &filePath); err != nil {
		return nil, false
	}
	info := buildSkillInfo(id.String(), skillName, slug, desc, version, s.baseDir, filePath)
	info.Visibility = visibility
	scanJSONStringArray(tagsJSON, &info.Tags)
	info.IsSystem = isSystem
	info.Author = parseFrontmatterAuthor(fmRaw)
	info.DisplayName, info.Family, info.Canonical, info.Replaces, info.Aliases = parseFrontmatterGovernance(fmRaw)
	return &info, true
}

func (s *SQLiteSkillStore) FilterSkills(ctx context.Context, allowList []string) []store.SkillInfo {
	all := s.ListSkills(ctx)
	var filtered []store.SkillInfo
	if allowList == nil {
		for _, sk := range all {
			if sk.Enabled {
				filtered = append(filtered, sk)
			}
		}
		return filtered
	}
	if len(allowList) == 0 {
		return nil
	}
	allowed := make(map[string]bool, len(allowList))
	for _, name := range allowList {
		allowed[name] = true
	}
	for _, sk := range all {
		if sk.Enabled && allowed[sk.Slug] {
			filtered = append(filtered, sk)
		}
	}
	return filtered
}

// GetSkillByID returns a SkillInfo for any skill by UUID regardless of status.
func (s *SQLiteSkillStore) GetSkillByID(ctx context.Context, id uuid.UUID) (store.SkillInfo, bool) {
	var name, slug, visibility, status string
	var desc *string
	var tagsJSON, depsRaw, fmRaw []byte
	var version int
	var isSystem, enabled bool
	var filePath *string
	q := `SELECT name, slug, description, visibility, tags, version, is_system, status, enabled, deps, frontmatter, file_path
		 FROM skills WHERE id = ?`
	args := []any{id}
	if !store.IsCrossTenant(ctx) {
		tid := store.TenantIDFromContext(ctx)
		if tid == uuid.Nil {
			tid = store.MasterTenantID
		}
		q += " AND (is_system = 1 OR tenant_id = ?)"
		args = append(args, tid)
	}
	if err := s.db.QueryRowContext(ctx, q, args...).Scan(&name, &slug, &desc, &visibility, &tagsJSON,
		&version, &isSystem, &status, &enabled, &depsRaw, &fmRaw, &filePath); err != nil {
		return store.SkillInfo{}, false
	}
	info := buildSkillInfo(id.String(), name, slug, desc, version, s.baseDir, filePath)
	info.Visibility = visibility
	scanJSONStringArray(tagsJSON, &info.Tags)
	info.IsSystem = isSystem
	info.Status = status
	info.Enabled = enabled
	info.MissingDeps = parseDepsColumn(depsRaw)
	info.Author = parseFrontmatterAuthor(fmRaw)
	info.DisplayName, info.Family, info.Canonical, info.Replaces, info.Aliases = parseFrontmatterGovernance(fmRaw)
	return info, true
}

func (s *SQLiteSkillStore) GetSkillOwnerID(ctx context.Context, id uuid.UUID) (string, bool) {
	q := "SELECT owner_id FROM skills WHERE id = ?"
	args := []any{id}
	if !store.IsCrossTenant(ctx) {
		tid := store.TenantIDFromContext(ctx)
		if tid == uuid.Nil {
			tid = store.MasterTenantID
		}
		q += " AND (is_system = 1 OR tenant_id = ?)"
		args = append(args, tid)
	}
	var ownerID string
	if err := s.db.QueryRowContext(ctx, q, args...).Scan(&ownerID); err != nil {
		return "", false
	}
	return ownerID, true
}

func (s *SQLiteSkillStore) GetSkillOwnerIDBySlug(ctx context.Context, slug string) (string, bool) {
	q := "SELECT owner_id FROM skills WHERE slug = ? AND status = 'active'"
	args := []any{slug}
	if !store.IsCrossTenant(ctx) {
		tid := store.TenantIDFromContext(ctx)
		if tid == uuid.Nil {
			tid = store.MasterTenantID
		}
		q += " AND (is_system = 1 OR tenant_id = ?)"
		args = append(args, tid)
	}
	var ownerID string
	if err := s.db.QueryRowContext(ctx, q, args...).Scan(&ownerID); err != nil {
		return "", false
	}
	return ownerID, true
}

// UpsertSystemSkill creates or updates a system skill.
// Returns (id, changed, actualFilePath, error).
func (s *SQLiteSkillStore) UpsertSystemSkill(ctx context.Context, p store.SkillCreateParams) (uuid.UUID, bool, string, error) {
	var existingID uuid.UUID
	var existingHash *string
	var existingFilePath string
	err := s.db.QueryRowContext(ctx,
		"SELECT id, file_hash, file_path FROM skills WHERE slug = ?", p.Slug,
	).Scan(&existingID, &existingHash, &existingFilePath)

	if err == nil {
		if existingHash != nil && p.FileHash != nil && *existingHash == *p.FileHash {
			return existingID, false, existingFilePath, nil
		}
		if existingHash == nil && p.FileHash != nil {
			_, _ = s.db.ExecContext(ctx,
				`UPDATE skills SET file_hash = ?, updated_at = ? WHERE id = ?`,
				p.FileHash, time.Now().UTC(), existingID,
			)
			return existingID, false, existingFilePath, nil
		}
		fmJSON := marshalFrontmatter(p.Frontmatter)
		_, err = s.db.ExecContext(ctx,
			`UPDATE skills SET name = ?, description = ?, version = ?, frontmatter = ?,
			 file_path = ?, file_size = ?, file_hash = ?, is_system = 1,
			 visibility = 'public', status = ?, updated_at = ?
			 WHERE id = ?`,
			p.Name, p.Description, p.Version, fmJSON,
			p.FilePath, p.FileSize, p.FileHash, p.Status, time.Now().UTC(), existingID,
		)
		if err != nil {
			return uuid.Nil, false, "", fmt.Errorf("update system skill: %w", err)
		}
		s.BumpVersion()
		return existingID, true, p.FilePath, nil
	}

	id := store.GenNewID()
	fmJSON := marshalFrontmatter(p.Frontmatter)
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO skills (id, name, slug, description, owner_id, visibility, version, status,
		 is_system, frontmatter, file_path, file_size, file_hash, tenant_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'system', 'public', ?, ?, 1, ?, ?, ?, ?, ?, ?, ?)`,
		id, p.Name, p.Slug, p.Description, p.Version, p.Status,
		fmJSON, p.FilePath, p.FileSize, p.FileHash, store.MasterTenantID, now, now,
	)
	if err != nil {
		return uuid.Nil, false, "", fmt.Errorf("insert system skill: %w", err)
	}
	s.BumpVersion()
	return id, true, p.FilePath, nil
}

// ListSystemSkillDirs returns slug->file_path map for all enabled system skills.
func (s *SQLiteSkillStore) ListSystemSkillDirs(ctx context.Context) map[string]string {
	rows, err := s.db.QueryContext(ctx,
		`SELECT slug, file_path FROM skills WHERE is_system = 1 AND enabled = 1`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	dirs := make(map[string]string)
	for rows.Next() {
		var slug, path string
		if err := rows.Scan(&slug, &path); err != nil {
			continue
		}
		dirs[slug] = path
	}
	if err := rows.Err(); err != nil {
		return nil
	}
	return dirs
}

// IsSystemSkill checks if a skill slug belongs to a system skill.
func (s *SQLiteSkillStore) IsSystemSkill(slug string) bool {
	var isSystem bool
	err := s.db.QueryRow("SELECT is_system FROM skills WHERE slug = ?", slug).Scan(&isSystem)
	return err == nil && isSystem
}

// IsCustomSkillSlug reports whether a slug already belongs to a custom skill in
// the current tenant. Seeders use this to avoid converting admin-managed skills
// back into system skills during restart.
func (s *SQLiteSkillStore) IsCustomSkillSlug(ctx context.Context, slug string) bool {
	tid := store.TenantIDFromContext(ctx)
	if tid == uuid.Nil {
		tid = store.MasterTenantID
	}
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM skills
			WHERE slug = ? AND tenant_id = ? AND is_system = 0 AND status != 'deleted'
		)`,
		slug, tid,
	).Scan(&exists)
	return err == nil && exists
}

package http

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/skills"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

var errSkillAddCoreFamilyCandidate = errors.New("skill family canonical is system-managed")

type skillAddApplyResult struct {
	Kind       string
	SkillID    uuid.UUID
	Slug       string
	Version    int
	OldVersion int
	OldPath    string
}

func (r skillAddApplyResult) needsPostflight() bool {
	return r.Kind == "created" || r.Kind == "merged"
}

// applySkillDraft creates a managed skill from a SuggestSkillAdd suggestion.
// Uses draftOverride if provided, otherwise falls back to the suggestion's parameters.skill_draft.
func (h *EvolutionHandler) applySkillDraft(ctx context.Context, sg store.EvolutionSuggestion, draftOverride, reviewedBy string) (skillAddApplyResult, error) {
	if h.skillStore == nil || h.skillLoader == nil {
		return skillAddApplyResult{}, fmt.Errorf("skill creation not available")
	}

	// Resolve draft content: request override > suggestion parameters.
	draft := draftOverride
	if draft == "" {
		var params map[string]any
		if err := json.Unmarshal(sg.Parameters, &params); err == nil {
			draft, _ = params["skill_draft"].(string)
		}
	}
	if draft == "" {
		return skillAddApplyResult{}, fmt.Errorf("no skill_draft content found")
	}

	// Security scan before any disk write.
	violations, safe := skills.GuardSkillContent(draft)
	if !safe {
		return skillAddApplyResult{}, fmt.Errorf("skill draft failed security scan: %s", skills.FormatGuardViolations(violations))
	}

	// Parse frontmatter for metadata.
	name, description, slug, frontmatter := skills.ParseSkillFrontmatter(draft)
	if name == "" {
		return skillAddApplyResult{}, fmt.Errorf("skill draft missing 'name' in frontmatter")
	}
	if slug == "" {
		slug = skills.Slugify(name)
	}
	if strings.TrimSpace(frontmatter["family"]) == "" {
		return skillAddApplyResult{}, fmt.Errorf("skill draft missing 'family' in frontmatter")
	}
	if existing := h.findCanonicalSkillByFamily(ctx, draft, slug); existing != nil {
		if existing.IsSystem {
			return skillAddApplyResult{}, h.markSkillAddCoreFamilyCandidate(ctx, sg, *existing, reviewedBy)
		}
		return h.applySkillDraftToExistingCustomSkill(ctx, sg, draft, *existing, reviewedBy)
	}

	// Resolve tenant-scoped destination directory.
	tenantID := store.TenantIDFromContext(ctx)
	tenantSlug := store.TenantSlugFromContext(ctx)
	baseDir := config.TenantSkillsStoreDir(h.dataDir, tenantID, tenantSlug)

	version := h.skillStore.GetNextVersion(ctx, slug)
	destDir := filepath.Join(baseDir, slug, fmt.Sprintf("%d", version))
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return skillAddApplyResult{}, fmt.Errorf("create skill directory: %w", err)
	}

	// Write SKILL.md file.
	contentBytes := []byte(draft)
	if err := os.WriteFile(filepath.Join(destDir, "SKILL.md"), contentBytes, 0644); err != nil {
		return skillAddApplyResult{}, fmt.Errorf("write SKILL.md: %w", err)
	}

	// DB insert.
	hasher := sha256.New()
	hasher.Write(contentBytes)
	fileHash := fmt.Sprintf("%x", hasher.Sum(nil))
	desc := description

	id, err := h.skillStore.CreateSkillManaged(ctx, store.SkillCreateParams{
		Name:        name,
		Slug:        slug,
		Description: &desc,
		OwnerID:     reviewedBy,
		Visibility:  "private",
		Version:     version,
		FilePath:    destDir,
		FileSize:    int64(len(contentBytes)),
		FileHash:    &fileHash,
		Frontmatter: frontmatter,
	})
	if err != nil {
		return skillAddApplyResult{}, fmt.Errorf("register skill: %w", err)
	}

	actualVersion := version
	actualPath := filepath.Join(destDir, "SKILL.md")
	if info, ok := h.skillStore.GetSkillByID(ctx, id); ok {
		if info.Version > 0 {
			actualVersion = info.Version
		}
		if info.Path != "" {
			actualPath = info.Path
		}
	}

	// Auto-grant generated custom skills to the affected agent so the next run can use them.
	if sg.AgentID != uuid.Nil {
		if err := h.skillStore.GrantToAgent(ctx, id, sg.AgentID, actualVersion, reviewedBy); err != nil {
			return skillAddApplyResult{}, fmt.Errorf("grant generated skill to agent: %w", err)
		}
	}

	// Bump loader to pick up new skill.
	h.skillLoader.BumpVersion()

	var params map[string]any
	_ = json.Unmarshal(sg.Parameters, &params)
	if params == nil {
		params = map[string]any{}
	}
	params["auto_applied"] = true
	params["skill_id"] = id.String()
	params["skill_slug"] = slug
	params["new_version"] = actualVersion
	params["new_file_path"] = actualPath
	params["applied_by"] = reviewedBy
	params["applied_at"] = time.Now().UTC().Format(time.RFC3339)
	updatedParams, _ := json.Marshal(params)
	if err := h.suggestions.UpdateSuggestionParameters(ctx, sg.ID, updatedParams); err != nil {
		return skillAddApplyResult{}, fmt.Errorf("record created skill parameters: %w", err)
	}

	// Mark suggestion as applied.
	if err := h.suggestions.UpdateSuggestionStatus(ctx, sg.ID, "applied", reviewedBy); err != nil {
		slog.Warn("evolution.skill_apply: status update failed", "error", err)
	}

	slog.Info("evolution.skill_apply: created", "skill_id", id, "slug", slug, "version", version, "suggestion", sg.ID)
	return skillAddApplyResult{
		Kind:    "created",
		SkillID: id,
		Slug:    slug,
		Version: actualVersion,
	}, nil
}

func (h *EvolutionHandler) applySkillDraftToExistingCustomSkill(ctx context.Context, sg store.EvolutionSuggestion, draft string, existing store.SkillInfo, reviewedBy string) (skillAddApplyResult, error) {
	if existing.IsSystem {
		return skillAddApplyResult{}, h.markSkillAddCoreFamilyCandidate(ctx, sg, existing, reviewedBy)
	}
	skillID, err := uuid.Parse(existing.ID)
	if err != nil {
		return skillAddApplyResult{}, fmt.Errorf("invalid skill id for %q: %w", existing.Slug, err)
	}
	currentBytes, err := os.ReadFile(existing.Path)
	if err != nil {
		return skillAddApplyResult{}, fmt.Errorf("read canonical custom skill: %w", err)
	}
	current := string(currentBytes)
	merged := mergeSkillAddDraftIntoCustomSkill(current, existing.Slug, draft)
	if merged == current {
		var params map[string]any
		_ = json.Unmarshal(sg.Parameters, &params)
		if params == nil {
			params = map[string]any{}
		}
		params["auto_applied"] = true
		params["no_change"] = true
		params["merged_into_skill"] = existing.Slug
		params["applied_by"] = reviewedBy
		params["applied_at"] = time.Now().UTC().Format(time.RFC3339)
		updatedParams, _ := json.Marshal(params)
		if err := h.suggestions.UpdateSuggestionParameters(ctx, sg.ID, updatedParams); err != nil {
			return skillAddApplyResult{}, fmt.Errorf("record no-change family merge parameters: %w", err)
		}
		if err := h.suggestions.UpdateSuggestionStatus(ctx, sg.ID, "applied", reviewedBy); err != nil {
			return skillAddApplyResult{}, fmt.Errorf("mark no-change family merge applied: %w", err)
		}
		return skillAddApplyResult{
			Kind:       "no_change",
			SkillID:    skillID,
			Slug:       existing.Slug,
			Version:    existing.Version,
			OldVersion: existing.Version,
			OldPath:    existing.Path,
		}, nil
	}
	violations, safe := skills.GuardSkillContent(merged)
	if !safe {
		return skillAddApplyResult{}, fmt.Errorf("family skill merge failed security scan: %s", skills.FormatGuardViolations(violations))
	}
	oldVersion := existing.Version
	oldPath := existing.Path
	newVersion, err := h.skillStore.SaveSkillContentVersion(ctx, skillID, merged)
	if err != nil {
		return skillAddApplyResult{}, fmt.Errorf("save family canonical skill version: %w", err)
	}
	if h.skillLoader != nil {
		h.skillLoader.BumpVersion()
	}
	if sg.AgentID != uuid.Nil {
		if err := h.skillStore.GrantToAgent(ctx, skillID, sg.AgentID, newVersion, reviewedBy); err != nil {
			return skillAddApplyResult{}, fmt.Errorf("grant merged canonical skill to agent: %w", err)
		}
	}
	var params map[string]any
	_ = json.Unmarshal(sg.Parameters, &params)
	if params == nil {
		params = map[string]any{}
	}
	params["auto_applied"] = true
	params["merged_into_skill"] = existing.Slug
	params["skill_id"] = skillID.String()
	params["old_version"] = oldVersion
	params["new_version"] = newVersion
	params["old_file_path"] = oldPath
	params["applied_by"] = reviewedBy
	params["applied_at"] = time.Now().UTC().Format(time.RFC3339)
	updatedParams, _ := json.Marshal(params)
	if err := h.suggestions.UpdateSuggestionParameters(ctx, sg.ID, updatedParams); err != nil {
		return skillAddApplyResult{}, fmt.Errorf("record family merge parameters: %w", err)
	}
	if err := h.suggestions.UpdateSuggestionStatus(ctx, sg.ID, "applied", reviewedBy); err != nil {
		return skillAddApplyResult{}, fmt.Errorf("mark family merge applied: %w", err)
	}
	slog.Info("evolution.skill_add: merged_into_family", "skill", existing.Slug, "old_version", oldVersion, "new_version", newVersion, "suggestion", sg.ID)
	return skillAddApplyResult{
		Kind:       "merged",
		SkillID:    skillID,
		Slug:       existing.Slug,
		Version:    newVersion,
		OldVersion: oldVersion,
		OldPath:    oldPath,
	}, nil
}

func (h *EvolutionHandler) markSkillAddCoreFamilyCandidate(ctx context.Context, sg store.EvolutionSuggestion, existing store.SkillInfo, reviewedBy string) error {
	var params map[string]any
	_ = json.Unmarshal(sg.Parameters, &params)
	if params == nil {
		params = map[string]any{}
	}
	params["auto_apply_blocked"] = true
	params["requires_manual_review"] = true
	params["blocked_reason"] = "family canonical skill is system-managed; automatic evolution only updates custom skills"
	params["family_canonical_skill"] = existing.Slug
	params["family_canonical_is_system"] = existing.IsSystem
	params["reviewed_by"] = reviewedBy
	params["reviewed_at"] = time.Now().UTC().Format(time.RFC3339)
	updatedParams, _ := json.Marshal(params)
	if err := h.suggestions.UpdateSuggestionParameters(ctx, sg.ID, updatedParams); err != nil {
		return fmt.Errorf("record core-family candidate parameters: %w", err)
	}
	return errSkillAddCoreFamilyCandidate
}

func (h *EvolutionHandler) rollbackSkillAddApplication(ctx context.Context, sg store.EvolutionSuggestion, result skillAddApplyResult, reviewedBy string) error {
	if h.skillStore == nil || h.skillLoader == nil || h.suggestions == nil {
		return fmt.Errorf("skill add rollback is not fully configured")
	}
	var params map[string]any
	_ = json.Unmarshal(sg.Parameters, &params)
	if params == nil {
		params = map[string]any{}
	}
	params["rolled_back"] = true
	params["rolled_back_by"] = reviewedBy
	params["rolled_back_at"] = time.Now().UTC().Format(time.RFC3339)
	params["rollback_kind"] = result.Kind
	params["rollback_version"] = result.Version

	switch result.Kind {
	case "created":
		if result.SkillID == uuid.Nil {
			return fmt.Errorf("skill add rollback missing skill_id")
		}
		if err := h.skillStore.DeleteSkill(ctx, result.SkillID); err != nil {
			return fmt.Errorf("delete created skill: %w", err)
		}
	case "merged":
		if result.SkillID == uuid.Nil {
			return fmt.Errorf("skill add rollback missing skill_id")
		}
		if result.OldPath == "" {
			return fmt.Errorf("skill add rollback missing old_file_path")
		}
		oldBytes, err := os.ReadFile(result.OldPath)
		if err != nil {
			return fmt.Errorf("read old skill content: %w", err)
		}
		oldContent := string(oldBytes)
		violations, safe := skills.GuardSkillContent(oldContent)
		if !safe {
			return fmt.Errorf("old skill content failed security scan: %s", skills.FormatGuardViolations(violations))
		}
		rollbackVersion, err := h.skillStore.SaveSkillContentVersion(ctx, result.SkillID, oldContent)
		if err != nil {
			return fmt.Errorf("save rollback skill version: %w", err)
		}
		params["rollback_version"] = rollbackVersion
		if h.skillLoader != nil {
			h.skillLoader.BumpVersion()
		}
	default:
		return nil
	}

	updatedParams, _ := json.Marshal(params)
	if err := h.suggestions.UpdateSuggestionParameters(ctx, sg.ID, updatedParams); err != nil {
		return fmt.Errorf("record skill add rollback parameters: %w", err)
	}
	if err := h.suggestions.UpdateSuggestionStatus(ctx, sg.ID, "rolled_back", reviewedBy); err != nil {
		return fmt.Errorf("mark skill add rolled back: %w", err)
	}
	return nil
}

func (h *EvolutionHandler) postflightSkillAddApplication(r *http.Request, agentID uuid.UUID, sg store.EvolutionSuggestion, result skillAddApplyResult, reviewedBy string) (string, error) {
	if !result.needsPostflight() {
		return "skill_add_applied", nil
	}
	postflight := h.executeRegressionRun(r, agentID, "business_workflow_smoke", sg.ID.String())
	if err := h.recordRegressionRun(r, agentID, postflight); err != nil {
		return "skill_add_postflight_record_failed", err
	}
	if postflight.Status != "passed" {
		if rollbackErr := h.rollbackSkillAddApplication(r.Context(), sg, result, reviewedBy); rollbackErr != nil {
			return "skill_add_postflight_failed_rollback_failed", rollbackErr
		}
		return "skill_add_postflight_failed_rolled_back", fmt.Errorf("skill add postflight failed; rollback version published")
	}
	return "skill_add_applied", nil
}

func mergeSkillAddDraftIntoCustomSkill(current, skillSlug, draft string) string {
	section := buildAutoEvolutionSection(skillSlug, draft)
	start := "<!-- GOCLAW_AUTO_EVOLUTION_START -->"
	end := "<!-- GOCLAW_AUTO_EVOLUTION_END -->"
	startIdx := strings.Index(current, start)
	endIdx := strings.Index(current, end)
	if startIdx >= 0 && endIdx > startIdx {
		endIdx += len(end)
		return strings.TrimRight(current[:startIdx], "\n") + "\n\n" + section + strings.TrimLeft(current[endIdx:], "\n")
	}
	return strings.TrimRight(current, "\n") + "\n\n" + section
}

func buildAutoEvolutionSection(skillSlug, draft string) string {
	var b strings.Builder
	b.WriteString("<!-- GOCLAW_AUTO_EVOLUTION_START -->\n")
	b.WriteString("## GoClaw Auto Evolution Notes\n\n")
	b.WriteString("This section is maintained automatically by GoClaw self-evolution for custom skills only. Keep the original workflow above and use these notes to improve the same family skill instead of creating parallel skills.\n\n")
	fmt.Fprintf(&b, "- Canonical custom skill: `%s`\n", skillSlug)
	b.WriteString("- Apply this family-level improvement before producing final output.\n")
	b.WriteString("- Do not create another skill for the same family unless no canonical custom skill exists.\n\n")
	b.WriteString("### Candidate Capability Draft\n\n")
	b.WriteString("```markdown\n")
	b.WriteString(strings.TrimSpace(draft))
	b.WriteString("\n```\n")
	b.WriteString("\n<!-- GOCLAW_AUTO_EVOLUTION_END -->\n")
	return b.String()
}

func (h *EvolutionHandler) findCanonicalSkillByFamily(ctx context.Context, content, slug string) *store.SkillInfo {
	if h.skillStore == nil {
		return nil
	}
	meta := skills.ParseSkillGovernance(content)
	family := meta.FamilyKey(slug)
	if family == "" {
		return nil
	}
	var systemCandidate *store.SkillInfo
	var customCandidate *store.SkillInfo
	for _, sk := range h.skillStore.ListSkills(ctx) {
		if sk.Status != "active" && sk.Status != "archived" {
			continue
		}
		skFamily := strings.TrimSpace(sk.Family)
		if skFamily == "" {
			skFamily = skills.Slugify(sk.Name)
		} else {
			skFamily = skills.Slugify(skFamily)
		}
		if skFamily == family {
			skCopy := sk
			if !sk.IsSystem {
				customCandidate = &skCopy
				continue
			}
			systemCandidate = &skCopy
		}
	}
	if customCandidate != nil {
		return customCandidate
	}
	return systemCandidate
}

func (h *EvolutionHandler) applySkillRepair(ctx context.Context, sg store.EvolutionSuggestion, reviewedBy string) error {
	if h.skillStore == nil || h.skillLoader == nil || h.suggestions == nil || h.metrics == nil {
		return fmt.Errorf("skill repair is not fully configured")
	}

	var params map[string]any
	if err := json.Unmarshal(sg.Parameters, &params); err != nil {
		return fmt.Errorf("parse repair parameters: %w", err)
	}
	slug, _ := params["skill"].(string)
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return fmt.Errorf("skill repair parameters missing skill")
	}

	info, ok := h.skillStore.GetSkill(ctx, slug)
	if !ok || info == nil {
		return fmt.Errorf("skill %q not found or not active", slug)
	}
	if info.IsSystem {
		return fmt.Errorf("skill %q is system-managed and cannot be auto-repaired", slug)
	}
	skillID, err := uuid.Parse(info.ID)
	if err != nil {
		return fmt.Errorf("invalid skill id for %q: %w", slug, err)
	}
	currentBytes, err := os.ReadFile(info.Path)
	if err != nil {
		return fmt.Errorf("read current SKILL.md: %w", err)
	}
	current := string(currentBytes)
	repaired, err := h.buildSkillRepairContent(ctx, sg, *info, current, params)
	if err != nil {
		return err
	}
	if repaired == current {
		params["auto_applied"] = true
		params["no_change"] = true
		params["applied_by"] = reviewedBy
		params["applied_at"] = time.Now().UTC().Format(time.RFC3339)
		updatedParams, _ := json.Marshal(params)
		if err := h.suggestions.UpdateSuggestionParameters(ctx, sg.ID, updatedParams); err != nil {
			return fmt.Errorf("record no-change skill repair parameters: %w", err)
		}
		if err := h.suggestions.UpdateSuggestionStatus(ctx, sg.ID, "applied", reviewedBy); err != nil {
			return fmt.Errorf("mark no-change skill repair applied: %w", err)
		}
		slog.Info("evolution.skill_repair: no_change", "skill", slug, "version", info.Version, "suggestion", sg.ID)
		return nil
	}
	violations, safe := skills.GuardSkillContent(repaired)
	if !safe {
		return fmt.Errorf("skill repair failed security scan: %s", skills.FormatGuardViolations(violations))
	}

	oldVersion := info.Version
	oldPath := info.Path
	newVersion, err := h.skillStore.SaveSkillContentVersion(ctx, skillID, repaired)
	if err != nil {
		return fmt.Errorf("save repaired skill version: %w", err)
	}
	if h.skillLoader != nil {
		h.skillLoader.BumpVersion()
	}

	params["auto_applied"] = true
	params["skill_id"] = skillID.String()
	params["old_version"] = oldVersion
	params["new_version"] = newVersion
	params["old_file_path"] = oldPath
	params["applied_by"] = reviewedBy
	params["applied_at"] = time.Now().UTC().Format(time.RFC3339)
	updatedParams, _ := json.Marshal(params)
	if err := h.suggestions.UpdateSuggestionParameters(ctx, sg.ID, updatedParams); err != nil {
		return fmt.Errorf("record repaired skill parameters: %w", err)
	}
	if err := h.suggestions.UpdateSuggestionStatus(ctx, sg.ID, "applied", reviewedBy); err != nil {
		return fmt.Errorf("mark skill repair applied: %w", err)
	}
	slog.Info("evolution.skill_repair: applied", "skill", slug, "old_version", oldVersion, "new_version", newVersion, "suggestion", sg.ID)
	return nil
}

func (h *EvolutionHandler) rollbackSkillRepair(ctx context.Context, sg store.EvolutionSuggestion, reviewedBy string) error {
	if h.skillStore == nil || h.skillLoader == nil || h.suggestions == nil {
		return fmt.Errorf("skill repair rollback is not fully configured")
	}
	var params map[string]any
	if err := json.Unmarshal(sg.Parameters, &params); err != nil {
		return fmt.Errorf("parse repair rollback parameters: %w", err)
	}
	if _, ok := params["skill_id"].(string); !ok && h.suggestions != nil {
		if refreshed, err := h.suggestions.GetSuggestion(ctx, sg.ID); err == nil && refreshed != nil {
			sg = *refreshed
			if err := json.Unmarshal(sg.Parameters, &params); err != nil {
				return fmt.Errorf("parse refreshed repair rollback parameters: %w", err)
			}
		}
	}
	skillIDRaw, _ := params["skill_id"].(string)
	if skillIDRaw == "" {
		return fmt.Errorf("skill repair rollback missing skill_id")
	}
	skillID, err := uuid.Parse(skillIDRaw)
	if err != nil {
		return fmt.Errorf("invalid skill repair rollback skill_id: %w", err)
	}
	oldPath, _ := params["old_file_path"].(string)
	if oldPath == "" {
		return fmt.Errorf("skill repair rollback missing old_file_path")
	}
	oldBytes, err := os.ReadFile(oldPath)
	if err != nil {
		return fmt.Errorf("read old skill content: %w", err)
	}
	oldContent := string(oldBytes)
	violations, safe := skills.GuardSkillContent(oldContent)
	if !safe {
		return fmt.Errorf("old skill content failed security scan: %s", skills.FormatGuardViolations(violations))
	}
	rollbackVersion, err := h.skillStore.SaveSkillContentVersion(ctx, skillID, oldContent)
	if err != nil {
		return fmt.Errorf("save rollback skill version: %w", err)
	}
	if h.skillLoader != nil {
		h.skillLoader.BumpVersion()
	}
	params["rolled_back"] = true
	params["rollback_version"] = rollbackVersion
	params["rolled_back_by"] = reviewedBy
	params["rolled_back_at"] = time.Now().UTC().Format(time.RFC3339)
	updatedParams, _ := json.Marshal(params)
	if err := h.suggestions.UpdateSuggestionParameters(ctx, sg.ID, updatedParams); err != nil {
		return fmt.Errorf("record rollback skill parameters: %w", err)
	}
	if err := h.suggestions.UpdateSuggestionStatus(ctx, sg.ID, "rolled_back", reviewedBy); err != nil {
		return fmt.Errorf("mark skill repair rolled back: %w", err)
	}
	slog.Info("evolution.skill_repair: rolled_back", "skill_id", skillID, "rollback_version", rollbackVersion, "suggestion", sg.ID)
	return nil
}

func (h *EvolutionHandler) buildSkillRepairContent(ctx context.Context, sg store.EvolutionSuggestion, info store.SkillInfo, current string, params map[string]any) (string, error) {
	insights, err := h.recentRepairFeedback(ctx, sg.AgentID, info.Slug)
	if err != nil {
		return "", err
	}
	section := buildAutoRepairSection(info.Slug, params, insights)
	start := "<!-- GOCLAW_AUTO_REPAIR_START -->"
	end := "<!-- GOCLAW_AUTO_REPAIR_END -->"
	startIdx := strings.Index(current, start)
	endIdx := strings.Index(current, end)
	if startIdx >= 0 && endIdx > startIdx {
		endIdx += len(end)
		return strings.TrimRight(current[:startIdx], "\n") + "\n\n" + section + strings.TrimLeft(current[endIdx:], "\n"), nil
	}
	return strings.TrimRight(current, "\n") + "\n\n" + section, nil
}

func (h *EvolutionHandler) recentRepairFeedback(ctx context.Context, agentID uuid.UUID, skillSlug string) ([]evolutionFeedbackValue, error) {
	since := time.Now().AddDate(0, 0, -90)
	metrics, err := h.metrics.QueryMetrics(ctx, agentID, store.MetricFeedback, since, 200)
	if err != nil {
		return nil, err
	}
	aliases := skillRepairAliases(skillSlug)
	var result []evolutionFeedbackValue
	for _, metric := range metrics {
		if metric.MetricKey == "useful" {
			continue
		}
		var value evolutionFeedbackValue
		if err := json.Unmarshal(metric.Value, &value); err != nil {
			continue
		}
		if value.FeedbackType != "correction" && value.FeedbackType != "not_useful" {
			continue
		}
		text := strings.ToLower(strings.Join([]string{value.MessageContent, value.Correction}, " "))
		if !containsAny(text, aliases) {
			continue
		}
		result = append(result, value)
		if len(result) >= 8 {
			break
		}
	}
	return result, nil
}

func buildAutoRepairSection(skillSlug string, params map[string]any, insights []evolutionFeedbackValue) string {
	var b strings.Builder
	b.WriteString("<!-- GOCLAW_AUTO_REPAIR_START -->\n")
	b.WriteString("## GoClaw Auto Repair Notes\n\n")
	b.WriteString("This section is maintained automatically by GoClaw self-evolution. Keep the original workflow above, and apply these repair rules before producing final output.\n\n")
	fmt.Fprintf(&b, "- Skill: `%s`\n", skillSlug)
	if score, ok := params["quality_score"]; ok {
		fmt.Fprintf(&b, "- Latest quality score: `%v`\n", score)
	}
	if risk, ok := params["risk_level"]; ok {
		fmt.Fprintf(&b, "- Risk level: `%v`\n", risk)
	}
	if feedback, ok := params["feedback_corrections"]; ok {
		fmt.Fprintf(&b, "- Correction/negative feedback count: `%v`\n", feedback)
	}
	if failures, ok := params["regression_failures"]; ok {
		fmt.Fprintf(&b, "- Regression failures: `%v`\n", failures)
	}
	b.WriteString("\n### Required Behavior\n\n")
	b.WriteString("- Before saying the task is complete, verify the actual output file or action result.\n")
	b.WriteString("- If source data is missing, report the missing source instead of inventing values.\n")
	b.WriteString("- Preserve existing user-visible output rules from the original skill.\n")
	b.WriteString("- Treat repeated user corrections below as higher priority than older assumptions.\n")
	if len(insights) > 0 {
		b.WriteString("\n### Recent User Corrections\n\n")
		for _, item := range insights {
			if item.MessageContent != "" {
				fmt.Fprintf(&b, "- Message: %s\n", normalizeRepairLine(item.MessageContent))
			}
			if item.Correction != "" {
				fmt.Fprintf(&b, "- Correction: %s\n", normalizeRepairLine(item.Correction))
			}
		}
	}
	b.WriteString("\n<!-- GOCLAW_AUTO_REPAIR_END -->\n")
	return b.String()
}

func skillRepairAliases(skillSlug string) []string {
	aliases := map[string][]string{
		"excel-type-identify":     {"excel-type-identify", "excel类型识别", "excel 类型识别"},
		"shipping-doc-processing": {"shipping-doc-processing", "船务清单处理", "船务清单", "ci", "epl"},
		"flow-order-query":        {"flow-order-query", "流转单查询", "流转单"},
		"package-weight-query":    {"package-weight-query", "产品包装重量查询", "产品包装重量", "重量表"},
		"label-generate":          {"label-generate", "标签生成", "标签"},
		"package-calculation":     {"package-calculation", "包装计算", "包装资料"},
	}
	names := aliases[skillSlug]
	if len(names) == 0 {
		names = []string{skillSlug}
	}
	return names
}

func containsAny(text string, aliases []string) bool {
	for _, alias := range aliases {
		if alias != "" && strings.Contains(text, strings.ToLower(alias)) {
			return true
		}
	}
	return false
}

func normalizeRepairLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		return s[:500] + "..."
	}
	return s
}

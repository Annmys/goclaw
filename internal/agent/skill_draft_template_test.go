package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/skills"
)

func TestGenerateSkillDraft_ContainsValidFrontmatter(t *testing.T) {
	draft := GenerateSkillDraft("web_search", 150, 0.85)

	name, desc, _, _ := skills.ParseSkillFrontmatter(draft)
	if name == "" {
		t.Fatal("draft frontmatter missing 'name' field")
	}
	if !strings.Contains(name, "web_search") {
		t.Errorf("name = %q, want to contain 'web_search'", name)
	}
	if desc == "" {
		t.Fatal("draft frontmatter missing 'description' field")
	}
}

func TestGenerateSkillDraft_IncludesToolMetrics(t *testing.T) {
	draft := GenerateSkillDraft("exec", 200, 0.92)

	if !strings.Contains(draft, "exec") {
		t.Error("draft missing tool name 'exec'")
	}
	if !strings.Contains(draft, "200") {
		t.Error("draft missing call count '200'")
	}
	if !strings.Contains(draft, "92%") {
		t.Error("draft missing success rate '92%'")
	}
}

func TestGenerateFeedbackSkillDraft_IncludesCorrections(t *testing.T) {
	draft := GenerateFeedbackSkillDraft("anchor", "Anchor", "auto-feedback-anchor", []FeedbackInsight{{
		CreatedAt:      time.Date(2026, 5, 21, 10, 30, 0, 0, time.UTC),
		FeedbackType:   "correction",
		SessionKey:     "session-1",
		MessageRef:     "message-1",
		MessageContent: "old answer",
		Correction:     "next time use the verified file path",
		UserID:         "system",
	}})

	name, desc, slug, _ := skills.ParseSkillFrontmatter(draft)
	if name != "Anchor 反馈记忆" {
		t.Fatalf("name = %q, want Anchor 反馈记忆", name)
	}
	if desc == "" {
		t.Fatal("draft frontmatter missing description")
	}
	if slug != "auto-feedback-anchor" {
		t.Fatalf("slug = %q, want auto-feedback-anchor", slug)
	}
	if !strings.Contains(draft, "next time use the verified file path") {
		t.Fatal("draft missing correction text")
	}
	if !strings.Contains(draft, "Prefer the most recent user correction") {
		t.Fatal("draft missing current rule guidance")
	}
}

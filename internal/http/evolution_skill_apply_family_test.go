package http

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type familySkillStoreStub struct {
	skills []store.SkillInfo
}

func (s *familySkillStoreStub) ListSkills(context.Context) []store.SkillInfo { return s.skills }
func (s *familySkillStoreStub) Dirs() []string                               { return []string{"."} }
func (s *familySkillStoreStub) CreateSkillManaged(context.Context, store.SkillCreateParams) (uuid.UUID, error) {
	return uuid.New(), nil
}
func (s *familySkillStoreStub) UpdateSkill(context.Context, uuid.UUID, map[string]any) error {
	return nil
}
func (s *familySkillStoreStub) DeleteSkill(context.Context, uuid.UUID) error       { return nil }
func (s *familySkillStoreStub) ToggleSkill(context.Context, uuid.UUID, bool) error { return nil }
func (s *familySkillStoreStub) LoadSkill(context.Context, string) (string, bool)   { return "", false }
func (s *familySkillStoreStub) LoadForContext(context.Context, []string) string    { return "" }
func (s *familySkillStoreStub) BuildSummary(context.Context, []string) string      { return "" }
func (s *familySkillStoreStub) GetSkill(context.Context, string) (*store.SkillInfo, bool) {
	return nil, false
}
func (s *familySkillStoreStub) FilterSkills(context.Context, []string) []store.SkillInfo {
	return s.skills
}
func (s *familySkillStoreStub) GetSkillByID(context.Context, uuid.UUID) (store.SkillInfo, bool) {
	return store.SkillInfo{}, false
}
func (s *familySkillStoreStub) GetSkillOwnerID(context.Context, uuid.UUID) (string, bool) {
	return "", false
}
func (s *familySkillStoreStub) GetSkillOwnerIDBySlug(context.Context, string) (string, bool) {
	return "", false
}
func (s *familySkillStoreStub) GetNextVersion(context.Context, string) int { return 1 }
func (s *familySkillStoreStub) GetNextVersionLocked(context.Context, string) (int, func() error, error) {
	return 1, func() error { return nil }, nil
}
func (s *familySkillStoreStub) SaveSkillContentVersion(context.Context, uuid.UUID, string) (int, error) {
	return 1, nil
}
func (s *familySkillStoreStub) GetSkillHashBySlug(context.Context, string) (string, int, bool) {
	return "", 0, false
}
func (s *familySkillStoreStub) IsCustomSkillSlug(context.Context, string) bool        { return false }
func (s *familySkillStoreStub) IsSystemSkill(string) bool                             { return false }
func (s *familySkillStoreStub) ListAllSkills(context.Context) []store.SkillInfo       { return s.skills }
func (s *familySkillStoreStub) ListAllSystemSkills(context.Context) []store.SkillInfo { return nil }
func (s *familySkillStoreStub) ListSystemSkillDirs(context.Context) map[string]string { return nil }
func (s *familySkillStoreStub) StoreMissingDeps(context.Context, uuid.UUID, []string) error {
	return nil
}
func (s *familySkillStoreStub) GrantToAgent(context.Context, uuid.UUID, uuid.UUID, int, string) error {
	return nil
}
func (s *familySkillStoreStub) RevokeFromAgent(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}
func (s *familySkillStoreStub) GrantToUser(context.Context, uuid.UUID, string, string) error {
	return nil
}
func (s *familySkillStoreStub) RevokeFromUser(context.Context, uuid.UUID, string) error { return nil }
func (s *familySkillStoreStub) ListWithGrantStatus(context.Context, uuid.UUID) ([]store.SkillWithGrantStatus, error) {
	return nil, nil
}
func (s *familySkillStoreStub) GetSkillFilePath(context.Context, uuid.UUID) (string, string, int, bool, bool) {
	return "", "", 0, false, false
}
func (s *familySkillStoreStub) Version() int64 { return 1 }
func (s *familySkillStoreStub) BumpVersion()   {}

type mockEvolutionSuggestionStore struct{}

func (m *mockEvolutionSuggestionStore) CreateSuggestion(context.Context, store.EvolutionSuggestion) error {
	return nil
}

func (m *mockEvolutionSuggestionStore) ListSuggestions(context.Context, uuid.UUID, string, int) ([]store.EvolutionSuggestion, error) {
	return nil, nil
}

func (m *mockEvolutionSuggestionStore) UpdateSuggestionStatus(context.Context, uuid.UUID, string, string) error {
	return nil
}

func (m *mockEvolutionSuggestionStore) UpdateSuggestionParameters(context.Context, uuid.UUID, json.RawMessage) error {
	return nil
}

func (m *mockEvolutionSuggestionStore) GetSuggestion(context.Context, uuid.UUID) (*store.EvolutionSuggestion, error) {
	return nil, nil
}

func TestFindCanonicalSkillByFamilyPrefersCustomSkill(t *testing.T) {
	h := &EvolutionHandler{
		skillStore: &familySkillStoreStub{
			skills: []store.SkillInfo{
				{
					ID:          uuid.NewString(),
					Slug:        "core-family",
					Name:        "Core Family",
					Status:      "active",
					Enabled:     true,
					IsSystem:    true,
					Family:      "demo-family",
					DisplayName: "Core Family",
				},
				{
					ID:          uuid.NewString(),
					Slug:        "custom-family",
					Name:        "Custom Family",
					Status:      "active",
					Enabled:     true,
					IsSystem:    false,
					Family:      "demo-family",
					DisplayName: "Custom Family",
				},
			},
		},
	}

	draft := "---\nname: Custom Family\nslug: custom-family\nfamily: demo-family\ncanonical: true\n---\nbody\n"
	sk := h.findCanonicalSkillByFamily(context.Background(), draft, "custom-family")
	if sk == nil {
		t.Fatal("expected family skill, got nil")
	}
	if sk.IsSystem {
		t.Fatalf("expected custom skill to be preferred, got system skill %q", sk.Slug)
	}
}

func TestMarkSkillAddCoreFamilyCandidate(t *testing.T) {
	h := &EvolutionHandler{
		suggestions: &mockEvolutionSuggestionStore{},
	}
	sg := store.EvolutionSuggestion{ID: uuid.New(), Parameters: []byte(`{"skill":"demo"}`)}
	err := h.markSkillAddCoreFamilyCandidate(context.Background(), sg, store.SkillInfo{
		Slug:     "core-family",
		Family:   "demo-family",
		IsSystem: true,
	}, "tester")
	if !errors.Is(err, errSkillAddCoreFamilyCandidate) {
		t.Fatalf("expected core family candidate error, got %v", err)
	}
}

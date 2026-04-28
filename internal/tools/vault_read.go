package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type VaultReadTool struct {
	vaultStore store.VaultStore
	workspace  string
}

func NewVaultReadTool() *VaultReadTool { return &VaultReadTool{} }

func (t *VaultReadTool) SetVaultStore(vs store.VaultStore) { t.vaultStore = vs }
func (t *VaultReadTool) SetKGStore(_ store.KnowledgeGraphStore) {}
func (t *VaultReadTool) SetEpisodicStore(_ store.EpisodicStore) {}
func (t *VaultReadTool) SetWorkspace(ws string) { t.workspace = ws }
func (t *VaultReadTool) Name() string           { return "vault_read" }

func (t *VaultReadTool) Description() string {
	return "Read full content of a vault document by doc_id returned from vault_search."
}

func (t *VaultReadTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"doc_id": map[string]any{
				"type":        "string",
				"description": "Vault document id from vault_search",
			},
		},
		"required": []string{"doc_id"},
	}
}

func (t *VaultReadTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.vaultStore == nil || t.workspace == "" {
		return ErrorResult("vault_read not available")
	}
	docID, _ := args["doc_id"].(string)
	docID = strings.TrimSpace(docID)
	if docID == "" {
		return ErrorResult("doc_id parameter is required")
	}
	tenantID := store.TenantIDFromContext(ctx).String()
	if tenantID == "" || tenantID == "00000000-0000-0000-0000-000000000000" {
		return ErrorResult("tenant not set in context")
	}
	doc, err := t.vaultStore.GetDocumentByID(ctx, tenantID, docID)
	if err != nil || doc == nil {
		return ErrorResult("document not found")
	}
	fullPath := filepath.Join(t.workspace, filepath.FromSlash(doc.Path))
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("read failed: %v", err))
	}
	return NewResult(string(data))
}

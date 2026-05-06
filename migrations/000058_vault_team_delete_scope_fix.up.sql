-- Fix team deletion with Vault team-scoped documents.
--
-- agent_teams deletion sets vault_documents.team_id to NULL. The old trigger
-- changed those rows to scope='personal', but personal scope requires agent_id
-- to be non-NULL. Team Vault docs have agent_id NULL, so deleting a team could
-- violate vault_documents_scope_consistency and fail. Demote orphaned team docs
-- to shared scope instead: the document remains accessible in the tenant and
-- the row stays valid.
CREATE OR REPLACE FUNCTION vault_docs_team_null_scope_fix()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.team_id IS NULL AND OLD.team_id IS NOT NULL AND NEW.scope = 'team' THEN
        NEW.scope := 'shared';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;


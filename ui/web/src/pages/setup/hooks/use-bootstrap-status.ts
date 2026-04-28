import { useMemo } from "react";
import { useProviders } from "@/pages/providers/hooks/use-providers";
import { useChatGPTOAuthProviderStatuses } from "@/pages/providers/hooks/use-chatgpt-oauth-provider-statuses";
import { useAgents } from "@/pages/agents/hooks/use-agents";
import { useAuthStore } from "@/stores/use-auth-store";
import { isSetupSkipped } from "@/lib/setup-skip";

export type SetupStep = 1 | 2 | 3 | 4 | "complete";

export function useBootstrapStatus() {
  const connected = useAuthStore((s) => s.connected);
  const userId = useAuthStore((s) => s.userId);
  const tenantId = useAuthStore((s) => s.tenantId);
  const tenantSlug = useAuthStore((s) => s.tenantSlug);
  const role = useAuthStore((s) => s.role);
  const isOwner = useAuthStore((s) => s.isOwner);
  const canManageSetup = isOwner || role === "admin";
  const { providers, loading: providersLoading } = useProviders(canManageSetup);
  const { statuses: oauthStatuses, isLoading: oauthStatusesLoading } =
    useChatGPTOAuthProviderStatuses(providers, canManageSetup);
  const { agents, loading: agentsLoading } = useAgents();

  // Wait for WS to connect before considering agents loaded
  const loading =
    agentsLoading ||
    (canManageSetup && (providersLoading || oauthStatusesLoading)) ||
    !connected;

  const { needsSetup, currentStep } = useMemo(() => {
    if (loading) return { needsSetup: false, currentStep: "complete" as SetupStep };

    // Setup is an admin concern. Paired tenant members often cannot read
    // provider configuration, so using their filtered view here incorrectly
    // redirects them back to the gateway setup page after login.
    if (!canManageSetup) {
      return { needsSetup: false, currentStep: "complete" as SetupStep };
    }

    const readyOAuthProviders = new Set(
      oauthStatuses
        .filter((status) => status.availability === "ready")
        .map((status) => status.provider.name),
    );
    const hasProvider = providers.some((provider) => provider.enabled && (
      provider.api_key === "***"
      || provider.provider_type === "claude_cli"
      || provider.provider_type === "ollama"
      || (provider.provider_type === "chatgpt_oauth" && readyOAuthProviders.has(provider.name))
    ));
    const hasAgent = agents.length > 0;

    // Allow skipping setup entirely via localStorage
    const skipped = isSetupSkipped({ userId, tenantId, tenantSlug });
    if (skipped) return { needsSetup: false, currentStep: "complete" as SetupStep };

    if (!hasProvider) return { needsSetup: true, currentStep: 1 as SetupStep };
    if (!hasAgent) return { needsSetup: true, currentStep: 2 as SetupStep };
    return { needsSetup: false, currentStep: "complete" as SetupStep };
  }, [agents, canManageSetup, loading, oauthStatuses, providers, tenantId, tenantSlug, userId]);

  return { needsSetup, currentStep, loading, providers, agents };
}

import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  AzureDevOpsConfig,
  AzureDevOpsOAuthAvailability,
  CompleteAzureDevOpsOAuthRequest,
  StartAzureDevOpsOAuthRequest,
  StartAzureDevOpsOAuthResponse,
} from "@/lib/types/azure-devops";
import { invalidateIntegrationAvailabilityAfter } from "@/lib/integrations/integration-availability-events";

const BASE = "/api/v1/azure-devops/oauth";

function withWorkspace(path: string, workspaceId: string): string {
  return `${path}?${new URLSearchParams({ workspace_id: workspaceId })}`;
}

export function getAzureDevOpsOAuthAvailability(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<AzureDevOpsOAuthAvailability> {
  return fetchJson<AzureDevOpsOAuthAvailability>(
    withWorkspace(`${BASE}/availability`, workspaceId),
    options,
  );
}

export function startAzureDevOpsOAuth(
  workspaceId: string,
  payload: StartAzureDevOpsOAuthRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<StartAzureDevOpsOAuthResponse>(withWorkspace(`${BASE}/start`, workspaceId), {
    ...options,
    init: { ...options?.init, method: "POST", body: JSON.stringify(payload) },
  });
}

export function completeAzureDevOpsOAuth(
  workspaceId: string,
  payload: CompleteAzureDevOpsOAuthRequest,
  options?: ApiRequestOptions,
) {
  return invalidateIntegrationAvailabilityAfter(
    fetchJson<AzureDevOpsConfig>(withWorkspace(`${BASE}/complete`, workspaceId), {
      ...options,
      init: { ...options?.init, method: "POST", body: JSON.stringify(payload) },
    }),
  );
}

export function signOutAzureDevOpsOAuth(workspaceId: string, options?: ApiRequestOptions) {
  return invalidateIntegrationAvailabilityAfter(
    fetchJson<AzureDevOpsConfig>(withWorkspace(`${BASE}/sign-out`, workspaceId), {
      ...options,
      init: { ...options?.init, method: "POST" },
    }),
  );
}

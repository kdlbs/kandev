import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  GitHubAutomationConnection,
  GitHubAppRegistration,
  GitHubAppRegistrationCatalog,
  GitHubCLIAccount,
  GitHubStatusResponse,
  ImportGitHubAppRegistrationRequest,
  PrepareGitHubAppImportRequest,
  PrepareGitHubAppImportResponse,
  StartGitHubAppManifestRequest,
  StartGitHubAppManifestResponse,
} from "@/lib/types/github";

export async function fetchGitHubStatus(workspaceId: string, options?: ApiRequestOptions) {
  const query = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<GitHubStatusResponse>(`/api/v1/github/status?${query}`, options);
}

export async function fetchGitHubCLIAccounts(workspaceId: string, options?: ApiRequestOptions) {
  const query = new URLSearchParams({ workspace_id: workspaceId });
  const response = await fetchJson<{ accounts: GitHubCLIAccount[] }>(
    `/api/v1/github/auth/gh-cli/accounts?${query}`,
    options,
  );
  return response.accounts ?? [];
}

export type SetGitHubConnectionRequest = {
  source: "pat" | "gh_cli";
  token?: string;
  host?: string;
  login?: string;
};

export async function setGitHubWorkspaceConnection(
  workspaceId: string,
  request: SetGitHubConnectionRequest,
) {
  const query = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<GitHubAutomationConnection>(`/api/v1/github/workspace-connection?${query}`, {
    init: { method: "PUT", body: JSON.stringify(request) },
  });
}

export async function disconnectGitHubWorkspace(workspaceId: string) {
  const query = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<{ disconnected: boolean }>(`/api/v1/github/workspace-connection?${query}`, {
    init: { method: "DELETE" },
  });
}

export type GitHubAuthStartResponse = { url?: string; URL?: string };

export async function startGitHubAppInstall(workspaceId: string, appRegistrationId: string) {
  return fetchJson<GitHubAuthStartResponse>("/api/v1/github/app/install/start", {
    init: {
      method: "POST",
      body: JSON.stringify({
        workspace_id: workspaceId,
        app_registration_id: appRegistrationId,
      }),
    },
  });
}

export async function startGitHubPersonalConnect(workspaceId: string) {
  return fetchJson<GitHubAuthStartResponse>("/api/v1/github/personal-connection/start", {
    init: { method: "POST", body: JSON.stringify({ workspace_id: workspaceId }) },
  });
}

export async function disconnectGitHubPersonal(workspaceId: string) {
  const query = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<{ disconnected: boolean }>(`/api/v1/github/personal-connection?${query}`, {
    init: { method: "DELETE" },
  });
}

export async function fetchGitHubAppRegistrations(
  workspaceId: string,
  options?: ApiRequestOptions,
) {
  const query = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<GitHubAppRegistrationCatalog>(`/api/v1/github/app/registrations?${query}`, {
    ...options,
    cache: options?.cache ?? "no-store",
  });
}

export async function startGitHubAppManifest(request: StartGitHubAppManifestRequest) {
  return fetchJson<StartGitHubAppManifestResponse>(
    "/api/v1/github/app/registrations/manifest/start",
    {
      init: { method: "POST", body: JSON.stringify(request) },
    },
  );
}

export async function importGitHubAppRegistration(request: ImportGitHubAppRegistrationRequest) {
  return fetchJson<GitHubAppRegistration>("/api/v1/github/app/registrations/import", {
    init: { method: "POST", body: JSON.stringify(request) },
  });
}

export async function prepareGitHubAppImport(request: PrepareGitHubAppImportRequest) {
  return fetchJson<PrepareGitHubAppImportResponse>(
    "/api/v1/github/app/registrations/import/prepare",
    {
      init: { method: "POST", body: JSON.stringify(request) },
    },
  );
}

export async function renameGitHubAppRegistration(registrationId: string, displayName: string) {
  return fetchJson<GitHubAppRegistration>(
    `/api/v1/github/app/registrations/${encodeURIComponent(registrationId)}`,
    {
      init: { method: "PATCH", body: JSON.stringify({ display_name: displayName }) },
    },
  );
}

export async function deleteGitHubAppRegistration(registrationId: string) {
  return fetchJson<{ deleted: boolean }>(
    `/api/v1/github/app/registrations/${encodeURIComponent(registrationId)}`,
    {
      init: { method: "DELETE" },
    },
  );
}

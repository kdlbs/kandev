import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  DeploymentGitHubAppStatus,
  GitHubAutomationConnection,
  GitHubCLIAccount,
  GitHubStatusResponse,
  StartDeploymentGitHubAppRequest,
  StartDeploymentGitHubAppResponse,
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

export async function startGitHubAppInstall(workspaceId: string) {
  return fetchJson<GitHubAuthStartResponse>("/api/v1/github/app/install/start", {
    init: { method: "POST", body: JSON.stringify({ workspace_id: workspaceId }) },
  });
}

export async function disconnectGitHubAppInstallation(workspaceId: string) {
  const query = new URLSearchParams({ workspace_id: workspaceId });
  return fetchJson<{ disconnected: boolean }>(`/api/v1/github/app/installation?${query}`, {
    init: { method: "DELETE" },
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

export async function fetchDeploymentAppRegistration(options?: ApiRequestOptions) {
  return fetchJson<DeploymentGitHubAppStatus>("/api/v1/github/app/registration", {
    ...options,
    cache: options?.cache ?? "no-store",
  });
}

export async function startDeploymentAppRegistration(request: StartDeploymentGitHubAppRequest) {
  return fetchJson<StartDeploymentGitHubAppResponse>("/api/v1/github/app/registration/start", {
    init: { method: "POST", body: JSON.stringify(request) },
  });
}

export async function deleteDeploymentAppRegistration() {
  return fetchJson<{ deleted: boolean }>("/api/v1/github/app/registration", {
    init: { method: "DELETE" },
  });
}

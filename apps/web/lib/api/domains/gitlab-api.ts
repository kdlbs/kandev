import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  GitLabStatus,
  GitLabConfigureTokenResponse,
  GitLabClearTokenResponse,
  GitLabConfigureHostResponse,
} from "@/lib/types/gitlab";

export async function fetchGitLabStatus(options?: ApiRequestOptions) {
  return fetchJson<GitLabStatus>("/api/v1/gitlab/status", options);
}

export async function configureGitLabToken(token: string) {
  return fetchJson<GitLabConfigureTokenResponse>("/api/v1/gitlab/token", {
    init: { method: "POST", body: JSON.stringify({ token }) },
  });
}

export async function clearGitLabToken() {
  return fetchJson<GitLabClearTokenResponse>("/api/v1/gitlab/token", {
    init: { method: "DELETE" },
  });
}

export async function configureGitLabHost(host: string) {
  return fetchJson<GitLabConfigureHostResponse>("/api/v1/gitlab/host", {
    init: { method: "POST", body: JSON.stringify({ host }) },
  });
}

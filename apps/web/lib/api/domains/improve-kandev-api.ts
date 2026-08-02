import { fetchJson, type ApiRequestOptions } from "../client";

// Result of the bootstrap fork-capability probe; mirrors backend ForkStatus.
// "writable": user has push access to upstream, no fork needed.
// "ready":    a fork already exists at github.com/{login}/kandev.
// "blocked_emu": user looks like an Enterprise Managed User and likely
//              cannot fork repositories outside their owning enterprise.
// "unknown":  bootstrap could not determine fork eligibility; proceed and
//             rely on the PR step to surface any errors.
export type ForkStatus = "writable" | "ready" | "blocked_emu" | "unknown";

export type ImproveKandevBootstrapResponse = {
  repository_id: string;
  workflow_id: string;
  issue_workflow_id: string;
  branch: string;
  bundle_dir: string;
  bundle_file: string;
  github_login: string;
  has_write_access: boolean;
  fork_status: ForkStatus;
  fork_message?: string;
};

export async function bootstrapImproveKandev(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<ImproveKandevBootstrapResponse> {
  return fetchJson<ImproveKandevBootstrapResponse>("/api/v1/system/improve-kandev/bootstrap", {
    ...options,
    init: {
      method: "POST",
      body: JSON.stringify({ workspace_id: workspaceId }),
      ...(options?.init ?? {}),
    },
  });
}

export async function leaseDiagnosticBundle(
  bundleDir: string,
  bundleId: string,
  options?: ApiRequestOptions,
): Promise<{ path: string; status: "ready" | "partial"; sources: string[] }> {
  return fetchJson<{ path: string; status: "ready" | "partial"; sources: string[] }>(
    "/api/v1/system/improve-kandev/bundle/lease",
    {
      ...options,
      init: {
        method: "POST",
        body: JSON.stringify({ bundle_dir: bundleDir, bundle_id: bundleId }),
        ...(options?.init ?? {}),
      },
    },
  );
}

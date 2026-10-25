import { fetchJson } from "../client";
import type { RepositoryCheckoutCapabilities } from "@/lib/types/repository-checkout-options";

export function getRepositoryCheckoutCapabilities(
  workspaceId: string,
  remoteUrl: string,
  provider: string | undefined,
  executorProfileId: string,
  signal: AbortSignal,
) {
  return fetchJson<RepositoryCheckoutCapabilities>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/repository-checkout-capabilities`,
    {
      init: {
        method: "POST",
        signal,
        body: JSON.stringify({
          repository: { remote_url: remoteUrl, provider },
          executor_profile_id: executorProfileId,
        }),
      },
    },
  );
}

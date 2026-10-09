import { useEffect, useState } from "react";
import { getRepositoryCheckoutCapabilities } from "@/lib/api/domains/repository-checkout-api";
import type { RepositoryCheckoutCapabilities } from "@/lib/types/repository-checkout-options";

type CapabilityState = {
  key: string;
  capabilities?: RepositoryCheckoutCapabilities;
  failed?: boolean;
};
export function useRepositoryCheckoutCapabilities(
  workspaceId: string | null | undefined,
  remoteUrl: string,
  provider: string | undefined,
  executorProfileId: string | undefined,
  { enabled, attempt = 0 }: { enabled: boolean; attempt?: number },
) {
  const key = JSON.stringify([workspaceId, remoteUrl, provider, executorProfileId, attempt]);
  const [state, setState] = useState<CapabilityState>();
  useEffect(() => {
    if (!enabled || !workspaceId || !remoteUrl) return;
    const controller = new AbortController();
    getRepositoryCheckoutCapabilities(
      workspaceId,
      remoteUrl,
      provider,
      executorProfileId ?? "",
      controller.signal,
    ).then(
      (capabilities) => {
        if (!controller.signal.aborted) setState({ key, capabilities });
      },
      () => {
        if (!controller.signal.aborted) setState({ key, failed: true });
      },
    );
    return () => controller.abort();
  }, [enabled, workspaceId, remoteUrl, provider, executorProfileId, key]);
  return state?.key === key ? state : undefined;
}

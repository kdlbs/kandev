import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import {
  readCachedProfileExecutionUsage,
  type ProfileExecutionUsage,
} from "@/lib/api/domains/provider-usage-api";
import { NativeControlContractError } from "@/lib/api/domains/control-native-api";
import { isCachedProfileUsage } from "@/lib/control-ui/usage";

/**
 * Reads one profile's stored execution observations on request. The cached
 * route never refreshes a provider, and a result from another login, backend
 * or profile is dropped instead of displayed.
 */
export function useProfileExecutionUsage(profileId: string) {
  const store = useAppStoreApi();
  const identity = useAppStore(stateReadScopeIdentity);
  const [snapshot, setSnapshot] = useState<{
    identity: string;
    profileId: string;
    entries: ProfileExecutionUsage[];
  }>();
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const request = useRef<AbortController | null>(null);
  const refresh = useCallback(async () => {
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    const current = () =>
      !controller.signal.aborted && stateReadScopeIdentity(store.getState()) === identity;
    setBusy(true);
    setFailed(false);
    setSnapshot(undefined);
    try {
      const result = await readCachedProfileExecutionUsage(profileId, {
        cache: "no-store",
        init: { signal: controller.signal },
      });
      if (!isCachedProfileUsage(result, profileId)) {
        // i18n-exempt: diagnostic only; the card displays the localized read error.
        throw new NativeControlContractError("Incompatible cached profile usage response");
      }
      if (current()) setSnapshot({ identity, profileId, entries: result.executions });
    } catch {
      if (current()) setFailed(true);
    } finally {
      if (current()) setBusy(false);
    }
  }, [profileId, identity, store]);
  useEffect(() => {
    setFailed(false);
    setBusy(false);
    return () => request.current?.abort();
  }, [refresh]);
  const entries =
    snapshot?.identity === identity && snapshot.profileId === profileId
      ? snapshot.entries
      : undefined;
  return { entries, busy, failed, refresh };
}

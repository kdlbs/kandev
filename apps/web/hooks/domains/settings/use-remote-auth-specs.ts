import { useEffect, useState } from "react";
import { listRemoteCredentials, type RemoteAuthSpec } from "@/lib/api/domains/settings-api";

let cached: Promise<RemoteAuthSpec[]> | null = null;

function loadAuthSpecs(): Promise<RemoteAuthSpec[]> {
  if (!cached) {
    cached = listRemoteCredentials()
      .then((res) => res.auth_specs ?? [])
      .catch(() => {
        cached = null;
        return [] as RemoteAuthSpec[];
      });
  }
  return cached;
}

/**
 * Module-cached fetch for remote-auth specs. Specs are static at runtime —
 * fetching once per page is enough. `loaded` lets callers defer gating until
 * the catalog is known, since "spec not in list" is a hard block once loaded.
 */
export function useRemoteAuthSpecs(): { specs: RemoteAuthSpec[]; loaded: boolean } {
  const [state, setState] = useState<{ specs: RemoteAuthSpec[]; loaded: boolean }>({
    specs: [],
    loaded: false,
  });
  useEffect(() => {
    let active = true;
    void loadAuthSpecs().then((s) => {
      if (active) setState({ specs: s, loaded: true });
    });
    return () => {
      active = false;
    };
  }, []);
  return state;
}

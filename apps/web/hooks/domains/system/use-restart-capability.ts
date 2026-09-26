"use client";

import { useEffect, useState } from "react";

import { fetchRestartCapability } from "@/lib/api/domains/system-api";
import type { RestartCapability } from "@/lib/types/system";

export type RestartCapabilityState =
  | { status: "loading"; capability: undefined }
  | { status: "resolved"; capability: RestartCapability }
  | { status: "unavailable"; capability: null };

export function useRestartCapability(): RestartCapabilityState {
  const [state, setState] = useState<RestartCapabilityState>({
    status: "loading",
    capability: undefined,
  });

  useEffect(() => {
    let cancelled = false;

    void fetchRestartCapability({ cache: "no-store" })
      .then((capability) => {
        if (!cancelled) setState({ status: "resolved", capability });
      })
      .catch(() => {
        if (!cancelled) setState({ status: "unavailable", capability: null });
      });

    return () => {
      cancelled = true;
    };
  }, []);

  return state;
}

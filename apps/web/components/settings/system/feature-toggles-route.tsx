"use client";

import { useEffect, useState } from "react";

import { fetchRestartCapability } from "@/lib/api/domains/system-api";
import type { RestartCapability } from "@/lib/types/system";

import { FeatureTogglesSettings } from "./feature-toggles-settings";

/**
 * Route wrapper that resolves restart capability for the Feature Toggles page.
 *
 * Capability is a property of the *running* backend — it depends on whether a
 * restart-capable supervisor launched it — so it must be fetched rather than
 * baked in at build time. Toggle cards render immediately; the capability
 * arrives after and only affects the pending-restart notice. A failed request
 * stays `null`, which falls back to manual restart guidance rather than
 * offering a button that cannot work.
 */
export function FeatureTogglesRoute() {
  const [restartCapability, setRestartCapability] = useState<RestartCapability | null>(null);

  useEffect(() => {
    let cancelled = false;

    fetchRestartCapability({ cache: "no-store" })
      .catch(() => null)
      .then((capability) => {
        if (!cancelled) setRestartCapability(capability);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  return <FeatureTogglesSettings initialFlags={[]} restartCapability={restartCapability} />;
}

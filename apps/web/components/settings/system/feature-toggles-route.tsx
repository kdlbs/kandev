"use client";

import { useRestartCapability } from "@/hooks/domains/system/use-restart-capability";
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
  const { capability: restartCapability } = useRestartCapability();

  return <FeatureTogglesSettings initialFlags={[]} restartCapability={restartCapability} />;
}

import { FeatureTogglesSettings } from "@/components/settings/system/feature-toggles-settings";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";
import { fetchRuntimeFlags } from "@/lib/api/domains/runtime-flags-api";
import { fetchRestartCapability } from "@/lib/api/domains/system-api";

export default async function FeatureTogglesPage() {
  const [flagsResponse, restartCapability] = await Promise.all([
    fetchRuntimeFlags({ cache: "no-store" }).catch(() => null),
    fetchRestartCapability({ cache: "no-store" }).catch(() => null),
  ]);

  return (
    <SystemPageShell
      title="Feature Toggles"
      description="Enable or disable experimental and diagnostic Kandev features."
    >
      <FeatureTogglesSettings
        initialFlags={flagsResponse?.flags ?? []}
        restartCapability={restartCapability}
      />
    </SystemPageShell>
  );
}

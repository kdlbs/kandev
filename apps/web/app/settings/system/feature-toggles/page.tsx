import { t } from "@/lib/i18n";
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
      title={t("system:navFeatureToggles")}
      description={t("system:featureTogglesPageDescription")}
    >
      <FeatureTogglesSettings
        initialFlags={flagsResponse?.flags ?? []}
        restartCapability={restartCapability}
      />
    </SystemPageShell>
  );
}

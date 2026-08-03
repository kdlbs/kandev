import { t } from "@/lib/i18n";
import { StateProvider } from "@/components/state-provider";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";
import { AboutCard } from "@/components/settings/system/about-card";
import { fetchSystemInfo } from "@/lib/api/domains/system-api";

export default async function SystemAboutPage() {
  let initialState: Record<string, unknown> = {};
  try {
    const info = await fetchSystemInfo({ cache: "no-store" }).catch(() => null);
    if (info) {
      initialState = { system: { info } };
    }
  } catch {
    initialState = {};
  }

  return (
    <StateProvider initialState={initialState}>
      <SystemPageShell title={t("system:navAbout")} description={t("system:aboutPageDescription")}>
        <AboutCard />
      </SystemPageShell>
    </StateProvider>
  );
}

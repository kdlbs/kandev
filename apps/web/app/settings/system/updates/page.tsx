import { StateProvider } from "@/components/state-provider";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";
import { UpdatesCard } from "@/components/settings/system/updates-card";
import { ChangelogList } from "@/components/settings/changelog-list";
import { fetchUpdates } from "@/lib/api/domains/system-api";
import { fetchUserSettings } from "@/lib/api";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";

export default async function SystemUpdatesPage() {
  let initialState: Record<string, unknown> = {};
  try {
    const [updates, settings] = await Promise.all([
      fetchUpdates({ cache: "no-store" }).catch(() => null),
      fetchUserSettings({ cache: "no-store" })
        .then(mapUserSettingsResponse)
        .catch(() => null),
    ]);
    initialState = {
      system: updates ? { updates } : undefined,
      userSettings: settings?.loaded ? settings : undefined,
    };
  } catch {
    initialState = {};
  }

  return (
    <StateProvider initialState={initialState}>
      <SystemPageShell
        title="Updates"
        description="Current vs latest release plus the full kandev changelog."
      >
        <UpdatesCard />
        <ChangelogList />
      </SystemPageShell>
    </StateProvider>
  );
}

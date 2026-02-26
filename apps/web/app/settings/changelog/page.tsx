import { ChangelogSettings } from "@/components/settings/changelog-settings";
import { StateProvider } from "@/components/state-provider";
import { fetchUserSettings } from "@/lib/api";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";

export default async function ChangelogPage() {
  let initialState = {};
  try {
    const response = await fetchUserSettings({ cache: "no-store" });
    const mapped = mapUserSettingsResponse(response);
    initialState = { userSettings: mapped.loaded ? mapped : undefined };
  } catch {
    initialState = {};
  }

  return (
    <StateProvider initialState={initialState}>
      <ChangelogSettings />
    </StateProvider>
  );
}

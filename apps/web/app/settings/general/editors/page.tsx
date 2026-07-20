import { EditorsSettings } from "@/components/settings/editors-settings";
import { StateHydrator } from "@/components/state-hydrator";
import { fetchUserSettings, listEditors } from "@/lib/api";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";

export default async function GeneralEditorsPage() {
  let initialState = {};
  try {
    const [editorsResponse, settingsResponse] = await Promise.all([
      listEditors({ cache: "no-store" }),
      fetchUserSettings({ cache: "no-store" }),
    ]);
    const mapped = mapUserSettingsResponse(settingsResponse);
    initialState = {
      editors: {
        items: editorsResponse.editors ?? [],
        loaded: true,
        loading: false,
      },
      userSettings: mapped.loaded ? mapped : undefined,
    };
  } catch {
    initialState = {};
  }

  return (
    <>
      <StateHydrator initialState={initialState} />
      <EditorsSettings />
    </>
  );
}

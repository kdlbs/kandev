import { fetchUserSettings } from "@/lib/api";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";

export async function getUserSettingsInitialState() {
  try {
    const response = await fetchUserSettings({ cache: "no-store" });
    const mapped = mapUserSettingsResponse(response);
    return { userSettings: mapped.loaded ? mapped : undefined };
  } catch {
    return {};
  }
}

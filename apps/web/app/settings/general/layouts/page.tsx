import { StateProvider } from "@/components/state-provider";
import { LayoutSettings } from "@/components/settings/layouts/layout-settings";
import { getUserSettingsInitialState } from "../user-settings-state";

export default async function GeneralLayoutsPage() {
  const initialState = await getUserSettingsInitialState();

  return (
    <StateProvider initialState={initialState}>
      <LayoutSettings />
    </StateProvider>
  );
}

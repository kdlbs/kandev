import { KeyboardShortcutsSettings } from "@/components/settings/general-settings";
import { StateProvider } from "@/components/state-provider";
import { getUserSettingsInitialState } from "../user-settings-state";

export default async function GeneralKeyboardShortcutsPage() {
  const initialState = await getUserSettingsInitialState();

  return (
    <StateProvider initialState={initialState}>
      <KeyboardShortcutsSettings />
    </StateProvider>
  );
}

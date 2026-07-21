import { TerminalSettings } from "@/components/settings/terminal-settings";
import { StateProvider } from "@/components/state-provider";
import { getUserSettingsInitialState } from "../user-settings-state";

export default async function GeneralTerminalPage() {
  const initialState = await getUserSettingsInitialState();

  return (
    <StateProvider initialState={initialState}>
      <TerminalSettings />
    </StateProvider>
  );
}

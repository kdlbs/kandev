import { ChatInputSettings } from "@/components/settings/general-settings";
import { StateProvider } from "@/components/state-provider";
import { getUserSettingsInitialState } from "../user-settings-state";

export default async function GeneralChatInputPage() {
  const initialState = await getUserSettingsInitialState();

  return (
    <StateProvider initialState={initialState}>
      <ChatInputSettings />
    </StateProvider>
  );
}

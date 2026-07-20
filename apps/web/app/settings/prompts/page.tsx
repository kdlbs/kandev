import { PromptsSettings } from "@/components/settings/prompts-settings";
import { StateHydrator } from "@/components/state-hydrator";
import { listPrompts } from "@/lib/api";

export default async function PromptsSettingsPage() {
  let initialState = {};
  try {
    const response = await listPrompts({ cache: "no-store" });
    initialState = {
      prompts: {
        items: response.prompts ?? [],
        loaded: true,
        loading: false,
      },
    };
  } catch {
    initialState = {};
  }

  return (
    <>
      <StateHydrator initialState={initialState} />
      <PromptsSettings />
    </>
  );
}

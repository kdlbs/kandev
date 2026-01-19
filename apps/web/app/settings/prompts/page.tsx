import { PromptsSettings } from '@/components/settings/prompts-settings';
import { StateHydrator } from '@/components/state-hydrator';
import { listPrompts } from '@/lib/http';
import type { CustomPrompt } from '@/lib/types/http';

export default async function PromptsSettingsPage() {
  let prompts: CustomPrompt[] = [];
  let initialState = {};
  try {
    const response = await listPrompts({ cache: 'no-store' });
    prompts = response.prompts ?? [];
    initialState = {
      prompts: {
        items: prompts,
        loaded: true,
        loading: false,
      },
    };
  } catch {
    prompts = [];
    initialState = {};
  }

  return (
    <>
      <StateHydrator initialState={initialState} />
      <PromptsSettings initialPrompts={prompts} />
    </>
  );
}

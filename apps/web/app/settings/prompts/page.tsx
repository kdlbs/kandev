import { PromptsSettings } from '@/components/settings/prompts-settings';
import { StateProvider } from '@/components/state-provider';
import { listPrompts } from '@/lib/http';

export default async function PromptsSettingsPage() {
  let initialState = {};
  try {
    const response = await listPrompts({ cache: 'no-store' });
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
    <StateProvider initialState={initialState}>
      <PromptsSettings />
    </StateProvider>
  );
}

import { EditorsSettings } from '@/components/settings/editors-settings';
import { fetchUserSettings, listEditors } from '@/lib/http';
import type { EditorOption, UserSettings } from '@/lib/types/http';

export default async function GeneralEditorsPage() {
  let editors: EditorOption[] = [];
  let settings: UserSettings | null = null;
  try {
    const [editorsResponse, settingsResponse] = await Promise.all([
      listEditors({ cache: 'no-store' }),
      fetchUserSettings({ cache: 'no-store' }),
    ]);
    editors = editorsResponse.editors ?? [];
    settings = settingsResponse.settings ?? null;
  } catch {
    editors = [];
    settings = null;
  }

  return <EditorsSettings initialEditors={editors} initialSettings={settings} />;
}

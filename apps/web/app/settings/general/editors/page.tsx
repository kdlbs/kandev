import { EditorsSettings } from '@/components/settings/editors-settings';
import { StateProvider } from '@/components/state-provider';
import { fetchUserSettings, listEditors } from '@/lib/http';

export default async function GeneralEditorsPage() {
  let initialState = {};
  try {
    const [editorsResponse, settingsResponse] = await Promise.all([
      listEditors({ cache: 'no-store' }),
      fetchUserSettings({ cache: 'no-store' }),
    ]);
    const settings = settingsResponse.settings;
    initialState = {
      editors: {
        items: editorsResponse.editors ?? [],
        loaded: true,
        loading: false,
      },
      userSettings: settings
        ? {
            workspaceId: settings.workspace_id ?? null,
            boardId: settings.board_id ?? null,
            repositoryIds: settings.repository_ids ?? [],
            preferredShell: settings.preferred_shell ?? null,
            shellOptions: settingsResponse.shell_options ?? [],
            defaultEditorId: settings.default_editor_id ?? null,
            loaded: true,
          }
        : undefined,
    };
  } catch {
    initialState = {};
  }

  return (
    <StateProvider initialState={initialState}>
      <EditorsSettings />
    </StateProvider>
  );
}

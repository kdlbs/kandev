import { GeneralSettings } from '@/components/settings/general-settings';
import { StateProvider } from '@/components/state-provider';
import { fetchUserSettings } from '@/lib/http';

export default async function GeneralSettingsPage() {
  let initialState = {};
  try {
    const response = await fetchUserSettings({ cache: 'no-store' });
    const settings = response.settings;
    initialState = {
      userSettings: settings
        ? {
            workspaceId: settings.workspace_id ?? null,
            boardId: settings.board_id ?? null,
            repositoryIds: settings.repository_ids ?? [],
            preferredShell: settings.preferred_shell ?? null,
            shellOptions: response.shell_options ?? [],
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
      <GeneralSettings />
    </StateProvider>
  );
}

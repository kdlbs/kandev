import { GeneralSettings } from "@/components/settings/general-settings";
import { StateProvider } from "@/components/state-provider";
import { fetchUserSettings } from "@/lib/api";

export default async function GeneralSettingsPage() {
  let initialState = {};
  try {
    const response = await fetchUserSettings({ cache: "no-store" });
    const settings = response.settings;
    initialState = {
      userSettings: settings
        ? {
            workspaceId: settings.workspace_id ?? null,
            workflowId: settings.workflow_filter_id ?? null,
            kanbanViewMode: settings.kanban_view_mode ?? null,
            repositoryIds: settings.repository_ids ?? [],
            preferredShell: settings.preferred_shell ?? null,
            shellOptions: response.shell_options ?? [],
            defaultEditorId: settings.default_editor_id ?? null,
            enablePreviewOnClick: settings.enable_preview_on_click ?? false,
            chatSubmitKey: settings.chat_submit_key ?? "cmd_enter",
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

import type { SidebarLayoutApi } from "../../lib/types/http-user-settings";
import type { ApiClient } from "./api-client";

export async function restoreSidebarLayout(
  apiClient: ApiClient,
  workspaceId: string,
  layout: SidebarLayoutApi | undefined,
): Promise<void> {
  const currentLayout = (await apiClient.getUserSettings()).settings.sidebar_layouts_by_workspace?.[
    workspaceId
  ];
  await apiClient.saveUserSettings({
    sidebar_layout_state: {
      workspace_id: workspaceId,
      expected_revision: currentLayout?.revision ?? 0,
      layout: layout ?? null,
    },
  });
}

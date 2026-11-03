import type { ApiClient } from "./api-client";

export const INTEGRATION_NODE = "plugin:kandev-plugin-e2e:e2e-integration";
export const INTEGRATION_LINK = "plugin-nav-item-e2e-integration";

export async function setIntegrationVisibility(
  api: ApiClient,
  workspaceId: string,
  visible: boolean,
  parent = false,
) {
  const { settings } = await api.getUserSettings();
  const saved = settings.sidebar_layouts_by_workspace![workspaceId];
  const nodes = [...saved.nodes];
  if (!parent && !nodes.some((node) => node.id === INTEGRATION_NODE)) {
    nodes.push({ id: INTEGRATION_NODE, kind: "plugin", visible, destination_id: INTEGRATION_NODE });
  }
  await api.saveUserSettings({
    sidebar_layout_state: {
      workspace_id: workspaceId,
      expected_revision: saved.revision,
      layout: {
        ...saved,
        nodes: nodes.map((node) =>
          node.id === (parent ? "integrations" : INTEGRATION_NODE) ? { ...node, visible } : node,
        ),
      },
    },
  });
}

export async function pinIntegration(api: ApiClient, workspaceId: string) {
  const { settings } = await api.getUserSettings();
  const saved = settings.sidebar_layouts_by_workspace![workspaceId];
  await api.saveUserSettings({
    sidebar_layout_state: {
      workspace_id: workspaceId,
      expected_revision: saved.revision,
      layout: {
        ...saved,
        nodes: [
          ...saved.nodes,
          {
            id: "pinned-integrations",
            kind: "shortcuts",
            visible: true,
            name: "Pinned integrations",
            shortcuts: [
              { id: "pinned-link", target: { kind: "destination", id: INTEGRATION_NODE } },
            ],
          },
        ],
      },
    },
  });
}

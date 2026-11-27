import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => {
  const workspaceId = "workspace-1";
  const coordinatorNodeId = "coordinators";
  const needsYouInboxNodeId = "needs-you-inbox";
  return {
    ids: { coordinatorNodeId, needsYouInboxNodeId },
    state: {
      workspaces: { activeId: workspaceId },
      userSettings: {
        sidebarLayoutsByWorkspace: {
          [workspaceId]: {
            version: 1,
            revision: 4,
            nodes: [
              { id: "home", kind: "builtin", visible: true, destination_id: "home" },
              { id: "inbox", kind: "builtin", visible: true, destination_id: "inbox" },
              {
                id: needsYouInboxNodeId,
                kind: "builtin",
                visible: false,
                destination_id: "needs_you_inbox",
              },
              {
                id: coordinatorNodeId,
                kind: "builtin",
                visible: true,
                destination_id: coordinatorNodeId,
              },
              { id: "canvases", kind: "builtin", visible: true, destination_id: "canvases" },
            ],
          },
        },
      },
    },
    flags: { coordinator: false, needsYouInbox: false, canvases: false } as Record<string, boolean>,
    officeMode: "kanban" as "office" | "kanban",
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));
vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: (feature: string) => mocks.flags[feature] ?? false,
}));
vi.mock("@/hooks/use-in-office", () => ({
  useOfficeModeState: () => mocks.officeMode,
}));
vi.mock("@/hooks/domains/sidebar/use-sidebar-shortcut-catalog", () => ({
  useSidebarShortcutCatalog: () => ({
    catalog: [],
    automations: [],
    definitionsLoading: false,
    definitionsError: null,
  }),
}));
vi.mock("@/hooks/domains/sidebar/use-shortcut-activity", () => ({
  useShortcutActivity: () => ({ getActivity: () => undefined, refresh: vi.fn() }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

import { useSidebarLayoutNavigation } from "./use-sidebar-layout-navigation";

describe("useSidebarLayoutNavigation", () => {
  beforeEach(() => {
    mocks.flags = { coordinator: false, needsYouInbox: false, canvases: false };
    mocks.officeMode = "kanban";
  });

  it("keeps disabled choices in the saved layout but excludes them from active navigation", () => {
    const { result } = renderHook(() => useSidebarLayoutNavigation());

    expect(result.current.layout.nodes.map((node) => node.id)).toEqual([
      "home",
      "inbox",
      mocks.ids.needsYouInboxNodeId,
      mocks.ids.coordinatorNodeId,
      "canvases",
    ]);
    expect(result.current.projection.nodes.map((node) => node.id)).not.toContain(
      mocks.ids.coordinatorNodeId,
    );
    expect(result.current.projection.nodes.map((node) => node.id)).not.toContain(
      mocks.ids.needsYouInboxNodeId,
    );
    expect(result.current.projection.nodes.map((node) => node.id)).not.toContain("canvases");
  });

  it("restores Coordinator and Canvases choices when their features become eligible", () => {
    mocks.flags = { coordinator: true, needsYouInbox: false, canvases: true };

    const { result } = renderHook(() => useSidebarLayoutNavigation());

    expect(result.current.projection.nodes.map((node) => node.id)).toEqual([
      "home",
      mocks.ids.coordinatorNodeId,
      "canvases",
    ]);
  });

  it("keeps Coordinator eligible in Office while applying Office-specific Inbox eligibility", () => {
    mocks.officeMode = "office";
    mocks.flags = { coordinator: true, needsYouInbox: true, canvases: true };

    const { result } = renderHook(() => useSidebarLayoutNavigation());

    expect(result.current.projection.nodes.map((node) => node.id)).toEqual([
      "home",
      "inbox",
      mocks.ids.needsYouInboxNodeId,
      mocks.ids.coordinatorNodeId,
    ]);
  });
});

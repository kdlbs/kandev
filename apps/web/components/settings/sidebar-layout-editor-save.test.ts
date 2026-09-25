import { describe, expect, it, vi } from "vitest";
import { defaultSidebarLayout, type SidebarLayout } from "@/lib/sidebar/layout-types";
import { submitSidebarLayout } from "./sidebar-layout-editor-save";

const mocks = vi.hoisted(() => ({ updateUserSettings: vi.fn() }));
const WORKSPACE_ID = "workspace-1";

vi.mock("@/lib/api/domains/settings-api", () => ({
  updateUserSettings: mocks.updateUserSettings,
  fetchUserSettings: vi.fn(),
}));

function layoutWithGroup(name: string): SidebarLayout {
  const layout = defaultSidebarLayout();
  layout.nodes.push({
    id: name.toLowerCase().replaceAll(" ", "-"),
    kind: "shortcuts",
    visible: true,
    name,
    shortcuts: [],
  });
  return layout;
}

describe("submitSidebarLayout", () => {
  it("keeps a response from a previous workspace out of the current saved baseline", async () => {
    let resolveSave: ((value: unknown) => void) | undefined;
    mocks.updateUserSettings.mockReturnValue(
      new Promise((resolve) => {
        resolveSave = resolve;
      }),
    );
    const submittedLayout = layoutWithGroup("Workspace one");
    const currentLayout = defaultSidebarLayout();
    const draftRef = { current: submittedLayout };
    const savedRef = { current: submittedLayout };
    const workspaceRef = { current: WORKSPACE_ID as string | null };
    const generationsRef = { current: new Map<string, number>() };
    const requestGenerationsRef = { current: new Map<string, number>() };
    const acknowledge = vi.fn();
    const setUserSettings = vi.fn();
    const onOperationError = vi.fn();
    const store = { getState: () => ({ userSettings: {} as never }) };
    const pending = submitSidebarLayout({
      catalog: [],
      acknowledge,
      setUserSettings,
      store,
      draftRef,
      savedRef,
      workspaceRef,
      generationsRef,
      requestGenerationsRef,
      onOperationError,
    });

    workspaceRef.current = "workspace-2";
    savedRef.current = currentLayout;
    draftRef.current = layoutWithGroup("Workspace two");
    await resolveSave?.({
      settings: {
        sidebar_layouts_by_workspace: {
          [WORKSPACE_ID]: {
            version: 1,
            revision: 1,
            nodes: [],
          },
        },
      },
    });
    await pending;

    expect(savedRef.current).toEqual(currentLayout);
    expect(draftRef.current.nodes.at(-1)?.name).toBe("Workspace two");
    expect(acknowledge).toHaveBeenCalledWith(
      WORKSPACE_ID,
      expect.objectContaining({ revision: 1 }),
      false,
    );
  });
});

describe("submission ordering", () => {
  it("ignores an older response after a newer save starts", async () => {
    const resolvers: Array<(value: unknown) => void> = [];
    mocks.updateUserSettings.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolvers.push(resolve);
        }),
    );
    const first = layoutWithGroup("First");
    const second = layoutWithGroup("Second");
    const draftRef = { current: first };
    const savedRef = { current: defaultSidebarLayout() };
    const workspaceRef = { current: WORKSPACE_ID as string | null };
    const generationsRef = { current: new Map<string, number>() };
    const requestGenerationsRef = { current: new Map<string, number>() };
    const acknowledge = vi.fn();
    const setUserSettings = vi.fn();
    const onOperationError = vi.fn();
    const store = { getState: () => ({ userSettings: {} as never }) };

    const firstSave = submitSidebarLayout({
      catalog: [],
      acknowledge,
      setUserSettings,
      store,
      draftRef,
      savedRef,
      workspaceRef,
      generationsRef,
      requestGenerationsRef,
      onOperationError,
    });
    draftRef.current = second;
    generationsRef.current.set(WORKSPACE_ID, 1);
    const secondSave = submitSidebarLayout({
      catalog: [],
      acknowledge,
      setUserSettings,
      store,
      draftRef,
      savedRef,
      workspaceRef,
      generationsRef,
      requestGenerationsRef,
      onOperationError,
    });

    resolvers[1]?.({
      settings: {
        sidebar_layouts_by_workspace: {
          [WORKSPACE_ID]: { version: 1, revision: 2, nodes: second.nodes },
        },
      },
    });
    await secondSave;
    resolvers[0]?.({
      settings: {
        sidebar_layouts_by_workspace: {
          [WORKSPACE_ID]: { version: 1, revision: 1, nodes: first.nodes },
        },
      },
    });
    await firstSave;

    expect(savedRef.current.revision).toBe(2);
    expect(draftRef.current.nodes.at(-1)?.name).toBe("Second");
    expect(acknowledge).toHaveBeenCalledTimes(1);
  });
});

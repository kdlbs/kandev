import { describe, expect, it, vi } from "vitest";
import { defaultSidebarLayout, type SidebarLayout } from "@/lib/sidebar/layout-types";
import { submitSidebarLayout } from "./sidebar-layout-editor-save";

const mocks = vi.hoisted(() => ({ updateUserSettings: vi.fn() }));

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
    const workspaceRef = { current: "workspace-1" as string | null };
    const generationsRef = { current: new Map<string, number>() };
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
      onOperationError,
    });

    workspaceRef.current = "workspace-2";
    savedRef.current = currentLayout;
    draftRef.current = layoutWithGroup("Workspace two");
    await resolveSave?.({
      settings: {
        sidebar_layouts_by_workspace: {
          "workspace-1": {
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
      "workspace-1",
      expect.objectContaining({ revision: 1 }),
      false,
    );
  });
});

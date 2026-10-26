import { act, cleanup, renderHook } from "@testing-library/react";
import type { DragEndEvent } from "@dnd-kit/core";
import { afterEach, expect, it, vi } from "vitest";
import { defaultSidebarLayout, toApiSidebarLayout } from "@/lib/sidebar/layout-types";
import type { ShortcutCatalogEntry } from "@/lib/sidebar/shortcut-catalog";
import { handleSidebarLayoutDragEnd } from "./sidebar-layout-editor-dnd";
import { useSidebarDraft, useSidebarLayoutActions } from "./sidebar-layout-editor-state";

const catalog: ShortcutCatalogEntry[] = ["plugin-a", "plugin-b"].map((id) => ({
  target: { kind: "destination", id },
  label: id,
  available: true,
  source: "plugin",
  section: "integrations",
}));
const saved = {
  ...defaultSidebarLayout(),
  nodes: [
    { id: "home", kind: "builtin" as const, visible: true, destinationId: "home" },
    { id: "integrations", kind: "builtin" as const, visible: true, destinationId: "integrations" },
    ...catalog.map((entry) => ({
      id: entry.target.id,
      kind: "plugin" as const,
      visible: true,
      destinationId: entry.target.id,
      pluginSection: "integrations" as const,
    })),
  ],
};
const state = {
  workspaces: { activeId: "workspace" },
  userSettings: { sidebarLayoutsByWorkspace: { workspace: toApiSidebarLayout(saved) } },
  setUserSettings: vi.fn(),
};
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
  useAppStoreApi: () => ({ getState: () => state }),
}));
afterEach(cleanup);

function useEditor() {
  const draft = useSidebarDraft(catalog);
  const actions = useSidebarLayoutActions({
    draft: draft.draft,
    workspaceId: draft.workspaceId,
    setDraft: draft.setDraft,
  });
  return { ...draft, apply: actions.applyDraftOperation };
}

function drag(from: string, to: string): DragEndEvent {
  return {
    active: { data: { current: { type: "node", nodeId: from } } },
    over: { data: { current: { type: "node", nodeId: to } } },
  } as unknown as DragEndEvent;
}

// @covers AC-UI-SIDEBAR-CUSTOMIZATION-001.11, AC-UI-SIDEBAR-CUSTOMIZATION-001.12
it.each([
  ["home", "plugin-a"],
  ["plugin-a", "home"],
])("keeps a clean Settings draft after dragging %s onto %s", (from, to) => {
  const { result } = renderHook(useEditor);
  const before = result.current.draft.nodes;
  act(() => {
    handleSidebarLayoutDragEnd(drag(from, to), result.current.draft, result.current.apply);
  });
  expect(result.current.draft.nodes).toEqual(before);
  expect(result.current.dirty).toBe(false);
});

it("marks a Settings draft dirty after a valid integration sibling move", () => {
  const { result } = renderHook(useEditor);
  expect(result.current.dirty).toBe(false);
  act(() => {
    handleSidebarLayoutDragEnd(
      drag("plugin-b", "plugin-a"),
      result.current.draft,
      result.current.apply,
    );
  });
  expect(
    result.current.draft.nodes.filter((node) => node.kind === "plugin").map((node) => node.id),
  ).toEqual(["plugin-b", "plugin-a"]);
  expect(result.current.dirty).toBe(true);
});

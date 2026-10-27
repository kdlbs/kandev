import { afterEach, expect, it, vi } from "vitest";
import { createStore, type StoreApi } from "zustand/vanilla";
import type { DockviewApi } from "dockview-react";
import type { AppState } from "@/lib/state/store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import {
  captureDockviewRename,
  isRenameReserved,
  renameScope,
  reserveRename,
  retargetFile,
  trackRenameOpen,
  waitForRenameRelease,
} from "./file-rename-continuity";

const ROOT_DESTINATION = "web/dest";
const DESTINATION_FILE = "dest/a.txt";

afterEach(() => {
  useDockviewStore.getState().clearFileStates();
  useDockviewStore.setState({ api: null });
  sessionStorage.clear();
});

// @covers AC-UI-FILE-EDITOR-MUTATION-002.2
it("maps repository-scoped descendants by segment and routes a move outside the repository to the workspace", () => {
  const file = { repo: "web", path: "src/file.txt", name: "file.txt" };
  expect(retargetFile(file, "web/src", "web/new")).toEqual({
    repo: "web",
    path: "new/file.txt",
    name: "file.txt",
  });
  expect(retargetFile(file, "web/src", "other/new")).toEqual({
    repo: undefined,
    path: "other/new/file.txt",
    name: "file.txt",
  });
  expect(retargetFile(file, "web/sr", "web/new")).toBeNull();
});

// @covers AC-UI-FILE-EDITOR-MUTATION-002.5
it("reserves only matching workspace paths, invalidates earlier opens, and releases its own token", async () => {
  const store = createStore(() => ({
    environmentIdBySessionId: { a: "env", b: "other" },
  })) as unknown as StoreApi<AppState>;
  const oldOpen = trackRenameOpen(store, "a", "src/a.txt", "web");
  const unrelated = trackRenameOpen(store, "a", "src-other/a.txt", "web");
  const release = reserveRename(store, renameScope(store, "a"), "web/src", ROOT_DESTINATION)!;
  expect(oldOpen.current()).toBe(false);
  expect(unrelated.current()).toBe(true);
  expect(isRenameReserved(store, "a", DESTINATION_FILE, "web")).toBe(true);
  expect(isRenameReserved(store, "b", DESTINATION_FILE, "web")).toBe(false);
  expect(reserveRename(store, renameScope(store, "a"), "web/dest/a.txt", "web/new")).toBeNull();
  let resumed = false;
  const waiting = waitForRenameRelease(store, "a", [{ path: DESTINATION_FILE, repo: "web" }]).then(
    () => {
      resumed = true;
    },
  );
  expect(resumed).toBe(false);
  release();
  await waiting;
  expect(resumed).toBe(true);
  expect(isRenameReserved(store, "a", DESTINATION_FILE, "web")).toBe(false);
  oldOpen.dispose();
  unrelated.dispose();
});

// @covers AC-UI-FILE-EDITOR-MUTATION-002.5
it("rejects an already-open destination even when its key uses a different repository alias", () => {
  const api = { getPanel: vi.fn() } as unknown as DockviewApi;
  useDockviewStore.setState({ api });
  useDockviewStore.getState().setFileState("web:new.txt", {
    path: "new.txt",
    repo: "web",
    name: "new.txt",
    content: "destination",
    originalContent: "disk",
    originalHash: "hash",
    isDirty: true,
  });
  expect(captureDockviewRename("web/old.txt", "web/new.txt", "a").collision).toBe(true);
  expect(useDockviewStore.getState().openFiles.get("web:new.txt")?.content).toBe("destination");
});

// @covers AC-UI-FILE-EDITOR-MUTATION-002.1
it("replaces a pinned panel in place without losing its buffer when removal cleanup runs", () => {
  type Group = { id: string; panels: Panel[]; activePanel?: Panel };
  type Panel = {
    id: string;
    params: Record<string, unknown>;
    group: Group;
    view: { contentComponent: string; tabComponent: string };
    toJSON: () => { renderer: "always" };
    setTitle: ReturnType<typeof vi.fn>;
    api: { setActive: () => void; updateParameters: ReturnType<typeof vi.fn> };
  };
  const panels = new Map<string, Panel>();
  let active: Panel | undefined;
  const group: Group = { id: "group", panels: [] };
  const makePanel = (id: string, params: Record<string, unknown>): Panel => {
    const panel: Panel = {
      id,
      params,
      group,
      view: { contentComponent: "file-editor", tabComponent: "pinnedDefaultTab" },
      toJSON: () => ({ renderer: "always" }),
      setTitle: vi.fn(),
      api: {
        setActive: () => {
          active = panel;
          group.activePanel = panel;
        },
        updateParameters: vi.fn(),
      },
    };
    return panel;
  };
  const before = makePanel("other", {});
  const source = makePanel("file:web:src/a.txt", { path: "src/a.txt", repo: "web", isDirty: true });
  const after = makePanel("after", {});
  group.panels = [before, source, after];
  for (const panel of group.panels) panels.set(panel.id, panel);
  source.api.setActive();
  const api = {
    get activePanel() {
      return active;
    },
    getPanel: (id: string) => panels.get(id),
    addPanel: (options: {
      id: string;
      params: Record<string, unknown>;
      position: { index: number };
    }) => {
      const panel = makePanel(options.id, options.params);
      group.panels.splice(options.position.index, 0, panel);
      panels.set(panel.id, panel);
      return panel;
    },
    removePanel: (panel: Panel) => {
      group.panels = group.panels.filter((item) => item !== panel);
      panels.delete(panel.id);
      useDockviewStore.getState().removeFileState(panel.id.slice("file:".length));
    },
  } as unknown as DockviewApi;
  useDockviewStore.setState({ api });
  useDockviewStore.getState().setFileState("web:src/a.txt", {
    path: "src/a.txt",
    repo: "web",
    name: "a.txt",
    content: "unsaved",
    originalContent: "disk",
    originalHash: "hash",
    isDirty: true,
  });
  const instance = useDockviewStore.getState().openFiles.get("web:src/a.txt")?.instanceId;
  const operation = captureDockviewRename("web/src/a.txt", "web/src/b.txt", "a");
  expect(operation.collision).toBe(false);
  operation.publish();
  expect(group.panels.map((panel) => panel.id)).toEqual(["other", "file:web:src/b.txt", "after"]);
  expect(active?.id).toBe("file:web:src/b.txt");
  expect(useDockviewStore.getState().openFiles.get("web:src/b.txt")).toMatchObject({
    content: "unsaved",
    originalContent: "disk",
    isDirty: true,
    instanceId: instance,
  });
  expect(useDockviewStore.getState().openFiles.has("web:src/a.txt")).toBe(false);
});

// @covers AC-UI-FILE-EDITOR-MUTATION-002.5
it("never overwrites a destination buffer that appears after rename admission", () => {
  const panel = {
    id: "preview:file-editor",
    params: { path: "old.txt", previewItemId: "old.txt" },
    api: { updateParameters: vi.fn() },
    setTitle: vi.fn(),
  };
  const api = {
    getPanel: (id: string) => (id === panel.id ? panel : undefined),
    panels: [],
  } as unknown as DockviewApi;
  useDockviewStore.setState({ api });
  const add = (path: string, content: string) =>
    useDockviewStore.getState().setFileState(path, {
      path,
      name: path,
      content,
      originalContent: "disk",
      originalHash: "hash",
      isDirty: true,
    });
  add("old.txt", "source draft");
  const operation = captureDockviewRename("old.txt", "new.txt", "a");
  expect(operation.collision).toBe(false);
  add("new.txt", "destination draft");
  expect(operation.publish()).toBe(false);
  expect(useDockviewStore.getState().openFiles.get("old.txt")?.content).toBe("source draft");
  expect(useDockviewStore.getState().openFiles.get("new.txt")?.content).toBe("destination draft");
});

it("retains the original dirty buffer if a pinned panel cannot be replaced", () => {
  const panel = {
    id: "file:old.txt",
    params: { path: "old.txt" },
    group: { id: "group", panels: [] as unknown[], activePanel: undefined },
    view: { contentComponent: "file-editor", tabComponent: "pinnedDefaultTab" },
    toJSON: () => ({}),
  };
  panel.group.panels = [panel];
  const api = {
    getPanel: (id: string) => (id === panel.id ? panel : undefined),
    addPanel: () => {
      throw new Error("host refused panel");
    },
    panels: [panel],
  } as unknown as DockviewApi;
  useDockviewStore.setState({ api });
  useDockviewStore.getState().setFileState("old.txt", {
    path: "old.txt",
    name: "old.txt",
    content: "unsaved",
    originalContent: "disk",
    originalHash: "hash",
    isDirty: true,
  });
  const result = captureDockviewRename("old.txt", "new.txt", "a");
  expect(result.collision).toBe(false);
  expect(result.publish()).toBe(false);
  expect(useDockviewStore.getState().openFiles.get("old.txt")?.content).toBe("unsaved");
  expect(useDockviewStore.getState().openFiles.has("new.txt")).toBe(false);
});

// @covers AC-UI-FILE-EDITOR-MUTATION-002.3
it("does not adopt a replacement preview panel even if the buffer object remains", () => {
  const original = {
    id: "preview:file-editor",
    params: { path: "old.txt", previewItemId: "old.txt" },
    view: { contentComponent: "file-editor" },
  };
  const replacement = { ...original, params: { ...original.params } };
  let preview = original;
  const api = {
    getPanel: (id: string) => (id === "preview:file-editor" ? preview : undefined),
    get panels() {
      return [preview];
    },
  } as unknown as DockviewApi;
  useDockviewStore.setState({ api });
  useDockviewStore.getState().setFileState("old.txt", {
    path: "old.txt",
    name: "old.txt",
    content: "draft",
    originalContent: "disk",
    originalHash: "hash",
    isDirty: true,
  });
  const operation = captureDockviewRename("old.txt", "new.txt", "a");
  expect(operation.collision).toBe(false);
  preview = replacement;
  expect(operation.publish()).toBe(true);
  expect(useDockviewStore.getState().openFiles.get("old.txt")?.content).toBe("draft");
  expect(useDockviewStore.getState().openFiles.has("new.txt")).toBe(false);
});

import { beforeEach, describe, expect, it } from "vitest";
import { useDockviewStore } from "./dockview-store";

const file = {
  path: "src/foo.ts",
  name: "foo.ts",
  content: "disk",
  originalContent: "disk",
  originalHash: "hash",
  isDirty: false,
};
beforeEach(() => useDockviewStore.getState().clearFileStates());

describe("file buffer lifetime", () => {
  // @covers AC-UI-FILE-EDITOR-MUTATION-001.2, AC-UI-FILE-EDITOR-MUTATION-001.3
  it("preserves lifetime during updates but replaces it on installation and reopen", () => {
    const store = useDockviewStore.getState();
    store.setFileState(file.path, file);
    const installed = useDockviewStore.getState().openFiles.get(file.path)!;
    expect(installed.instanceId).toBeDefined();
    store.updateFileState(file.path, { content: "typed", isDirty: true });
    expect(useDockviewStore.getState().openFiles.get(file.path)?.instanceId).toBe(
      installed.instanceId,
    );
    store.setFileState(file.path, installed);
    expect(useDockviewStore.getState().openFiles.get(file.path)?.instanceId).not.toBe(
      installed.instanceId,
    );
    const previousInstanceId = useDockviewStore.getState().openFiles.get(file.path)?.instanceId;
    store.removeFileState(file.path);
    store.setFileState(file.path, installed);
    expect(useDockviewStore.getState().openFiles.get(file.path)?.instanceId).not.toBe(
      previousInstanceId,
    );
    store.clearFileStates();
    expect(useDockviewStore.getState().openFiles.size).toBe(0);
  });
});

// @covers AC-UI-MARKDOWN-FILE-EDITING-003.8
it("reuses a retained canonical record and retires only the selected instance", () => {
  const store = useDockviewStore.getState();
  store.activateFileSession("registry-A");
  store.setFileState(file.path, file);
  store.updateFileState(file.path, { content: "draft", isDirty: true });
  const retained = useDockviewStore.getState().openFiles.get(file.path);
  store.clearFileStates();
  store.activateFileSession("registry-B");
  store.setFileState(file.path, { ...file, content: "other repository session" });
  store.activateFileSession("registry-A");
  expect(useDockviewStore.getState().openFiles.get(file.path)).toBe(retained);
  store.ensureFileState(file.path, file);
  expect(useDockviewStore.getState().openFiles.get(file.path)).toBe(retained);
  store.removeFileState(file.path);
  store.activateFileSession("registry-B");
  expect(useDockviewStore.getState().openFiles.get(file.path)?.content).toBe(
    "other repository session",
  );
  store.activateFileSession("registry-A");
  expect(useDockviewStore.getState().openFiles.has(file.path)).toBe(false);
});

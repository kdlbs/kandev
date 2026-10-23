import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { FileDraftSessionContext, type FileDraftSessionOwner } from "../file-draft-session-context";
import { useMobileFileSave } from "./mobile-file-save";

const mocks = vi.hoisted(() => ({ save: vi.fn(), toast: vi.fn(), client: {} }));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => mocks.client }));
vi.mock("@/lib/ws/workspace-files", () => ({
  updateFileContent: (...args: unknown[]) => mocks.save(...args),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: mocks.toast }) }));
const file = {
  path: "README.md",
  name: "README.md",
  content: "B",
  originalContent: "A",
  originalHash: "hash-A",
  isDirty: true,
};
let owner: FileDraftSessionOwner;
function wrapper({ children }: { children: ReactNode }) {
  return (
    <FileDraftSessionContext.Provider value={owner}>{children}</FileDraftSessionContext.Provider>
  );
}
function mountSave() {
  return renderHook(
    () => {
      const draft = useDockviewStore((state) => state.openFiles.get(file.path))!;
      return useMobileFileSave({
        file: draft,
        sessionId: "phone-A",
        fileIdentity: file.path,
        draftContent: draft.content,
        baselineContent: draft.originalContent,
        originalHash: draft.originalHash,
        isDirty: draft.isDirty,
        markSaved: vi.fn(),
      });
    },
    { wrapper },
  );
}
function deferred() {
  let resolve!: (value: { success: boolean; new_hash: string }) => void;
  const promise = new Promise<{ success: boolean; new_hash: string }>((yes) => {
    resolve = yes;
  });
  return { promise, resolve };
}
beforeEach(() => {
  vi.clearAllMocks();
  owner = { sessionIdRef: { current: "phone-A" }, visitRef: { current: Symbol() } };
  useDockviewStore.setState({
    openFiles: new Map(),
    fileDrafts: new Map(),
    mobileFileKeys: new Map(),
    activeFileSessionId: null,
  });
  const store = useDockviewStore.getState();
  store.activateFileSession("phone-A");
  store.setFileState(file.path, file);
});
afterEach(cleanup);

// @covers AC-UI-MARKDOWN-FILE-EDITING-002.14, AC-UI-MARKDOWN-FILE-EDITING-003.8
it("accepts a phone save after responsive unmount while preserving newer typing", async () => {
  const hook = mountSave();
  const reply = deferred();
  mocks.save.mockReturnValue(reply.promise);
  let pending!: Promise<void>;
  act(() => {
    pending = hook.result.current.handleSave();
  });
  hook.unmount();
  act(() =>
    useDockviewStore.getState().updateFileState(file.path, { content: "C", isDirty: true }),
  );
  await act(async () => {
    reply.resolve({ success: true, new_hash: "hash-B" });
    await pending;
  });
  expect(useDockviewStore.getState().openFiles.get(file.path)).toMatchObject({
    content: "C",
    originalContent: "B",
    originalHash: "hash-B",
    isDirty: true,
  });
});

it.each(["replacement", "session revisit", "owner disposal"])(
  "rejects a phone save after %s",
  async (retirement) => {
    const hook = mountSave();
    const reply = deferred();
    mocks.save.mockReturnValue(reply.promise);
    let pending!: Promise<void>;
    act(() => {
      pending = hook.result.current.handleSave();
    });
    act(() => {
      const store = useDockviewStore.getState();
      if (retirement === "replacement")
        store.setFileState(file.path, { ...file, content: "replacement" });
      else if (retirement === "session revisit") {
        store.activateFileSession("phone-B");
        store.activateFileSession("phone-A");
      } else owner.visitRef.current = null;
    });
    const current = useDockviewStore.getState().openFiles.get(file.path);
    await act(async () => {
      reply.resolve({ success: true, new_hash: "obsolete" });
      await pending;
    });
    expect(useDockviewStore.getState().openFiles.get(file.path)).toBe(current);
    expect(current?.originalHash).toBe("hash-A");
  },
);

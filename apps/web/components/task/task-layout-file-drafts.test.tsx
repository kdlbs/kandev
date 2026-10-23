import { act, cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { buildRepoScopedItemId } from "@/lib/state/dockview-panel-actions";
import { requestNavigation, clearNavigationBlockerForTests } from "@/lib/routing/navigation-guard";
import { setOpenFileTabs } from "@/lib/local-storage";
import * as workspaceSync from "@/hooks/file-editors-sync";
import { useFileEditors } from "@/hooks/use-file-editors";
import { useMobilePanelHandlers } from "./mobile/session-mobile-layout";
import { MobileNavigationGuardProvider } from "./mobile/mobile-navigation-guard";
import { TaskCenterPanel } from "./task-center-panel";
import { FileDraftSessionProvider } from "./file-draft-session-provider";

const mocks = vi.hoisted(() => ({
  client: {
    on: vi.fn<
      (
        type: string,
        handler: (message: {
          payload: {
            session_id: string;
            changes: { path: string; operation: string; repository_name?: string }[];
          };
        }) => void,
      ) => () => void
    >(() => () => undefined),
  },
  save: vi.fn(),
  read: vi.fn(),
  toast: vi.fn(),
  tablet: null as null | {
    tab: { content: string; instanceId?: symbol };
    onFileChange: (path: string, content: string, repo?: string) => void;
    onFileSave: (path: string, repo?: string) => Promise<void>;
  },
  dialog: null as null | { open: boolean; onCancel: () => void; onConfirm: () => void },
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: mocks.toast }) }));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => mocks.client }));
vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileContent: (...args: unknown[]) => mocks.read(...args),
  updateFileContent: (...args: unknown[]) => mocks.save(...args),
  deleteFile: vi.fn(),
}));
vi.mock("@/hooks/domains/session/use-session-git-status", () => ({
  useSessionGitStatus: () => undefined,
}));
vi.mock("@/lib/lsp/lsp-client-manager", () => ({ lspClientManager: { saveDocument: vi.fn() } }));
vi.mock("@/components/editors/monaco/monaco-init", () => ({
  getMonacoInstance: () => null,
  initMonacoThemes: vi.fn(),
}));
vi.mock("@/components/discard-local-changes-dialog", () => ({
  DiscardLocalChangesDialog: (props: NonNullable<typeof mocks.dialog>) => {
    mocks.dialog = props;
    return null;
  },
}));

vi.mock("@/hooks/domains/session/use-session-commits", () => ({
  useSessionCommits: () => ({ commits: [] }),
}));
vi.mock("./review-panel-provider", () => ({ useNormalizedTaskReviews: () => [] }));
vi.mock("./task-chat-panel", () => ({ TaskChatPanel: () => null }));
vi.mock("./task-changes-panel", () => ({ TaskChangesPanel: () => null }));
vi.mock("./file-tab-content", () => ({
  FileTabContent: (props: NonNullable<typeof mocks.tablet>) => {
    mocks.tablet = props;
    return null;
  },
}));
function Tablet() {
  return (
    <TaskCenterPanel
      selectedDiff={null}
      openFileRequest={null}
      onDiffHandled={() => undefined}
      onFileOpenHandled={() => undefined}
    />
  );
}

const file = {
  path: "README.md",
  name: "README.md",
  content: "A",
  originalContent: "A",
  originalHash: "hash-A",
  isDirty: false,
};
let mobile: ReturnType<typeof useMobilePanelHandlers>;
let desktop: ReturnType<typeof useFileEditors>;
let app: ReturnType<typeof useAppStoreApi>;
function CaptureStore() {
  app = useAppStoreApi();
  return null;
}
function Mobile() {
  const sessionId = app.getState().tasks.activeSessionId;
  mobile = useMobilePanelHandlers({
    effectiveSessionId: sessionId,
    handlePanelChange: () => undefined,
  });
  return null;
}
function Desktop() {
  desktop = useFileEditors();
  return null;
}
function Owner({
  children,
  initialSessionId = "draft-A",
}: {
  children: ReactNode;
  initialSessionId?: string;
}) {
  return (
    <StateProvider
      initialState={{
        tasks: {
          activeTaskId: null,
          pinnedSessionId: null,
          lastSessionByTaskId: {},
          resumeSkippedSessionIds: {},
          activeSessionId: initialSessionId,
        },
      }}
    >
      <CaptureStore />
      <MobileNavigationGuardProvider>
        <FileDraftSessionProvider>{children}</FileDraftSessionProvider>
      </MobileNavigationGuardProvider>
    </StateProvider>
  );
}
function current(repo?: string) {
  return useDockviewStore.getState().openFiles.get(buildRepoScopedItemId(file.path, repo));
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
  localStorage.clear();
  sessionStorage.clear();
  useDockviewStore.setState({
    api: null,
    openFiles: new Map(),
    fileDrafts: new Map(),
    mobileFileKeys: new Map(),
    activeFileSessionId: null,
  });
});
afterEach(() => {
  cleanup();
  clearNavigationBlockerForTests();
  vi.restoreAllMocks();
});

// @covers AC-UI-MARKDOWN-FILE-EDITING-003.8, AC-UI-MARKDOWN-FILE-EDITING-002.3
it("retains and saves one draft across the 768px boundary", async () => {
  const view = render(
    <Owner>
      <Mobile />
    </Owner>,
  );
  act(() => mobile.handleOpenFile(file));
  act(() => mobile.handleSelectedFileChange("B"));
  const instance = current()?.instanceId;
  view.rerender(
    <Owner>
      <Desktop />
    </Owner>,
  );
  expect(current()).toMatchObject({
    content: "B",
    originalContent: "A",
    isDirty: true,
    instanceId: instance,
  });
  act(() => desktop.handleFileChange(file.path, "C"));
  view.rerender(
    <Owner>
      <Mobile />
    </Owner>,
  );
  expect(mobile.selectedFile).toBe(current());
  expect(mobile.selectedFile?.content).toBe("C");
  view.rerender(
    <Owner>
      <Desktop />
    </Owner>,
  );
  mocks.save.mockResolvedValue({ success: true, new_hash: "hash-C" });
  await act(async () => desktop.saveFile(file.path));
  expect(current()).toMatchObject({
    content: "C",
    originalContent: "C",
    originalHash: "hash-C",
    isDirty: false,
  });
  expect(mocks.save).toHaveBeenCalledWith(
    mocks.client,
    "draft-A",
    expect.objectContaining({ desiredContent: "C", originalHash: "hash-A" }),
  );
});

// @covers AC-UI-MARKDOWN-FILE-EDITING-002.14, AC-UI-MARKDOWN-FILE-EDITING-003.8
it("accepts an owned save after responsive unmount and preserves newer typing", async () => {
  const view = render(
    <Owner>
      <Mobile />
    </Owner>,
  );
  act(() => mobile.handleOpenFile(file));
  act(() => mobile.handleSelectedFileChange("B"));
  view.rerender(
    <Owner>
      <Desktop />
    </Owner>,
  );
  const reply = deferred();
  mocks.save.mockReturnValue(reply.promise);
  let pending!: Promise<void>;
  act(() => {
    pending = desktop.saveFile(file.path);
  });
  view.rerender(
    <Owner>
      <Mobile />
    </Owner>,
  );
  act(() => mobile.handleSelectedFileChange("C"));
  await act(async () => {
    reply.resolve({ success: true, new_hash: "hash-B" });
    await pending;
  });
  expect(current()).toMatchObject({
    content: "C",
    originalContent: "B",
    originalHash: "hash-B",
    isDirty: true,
  });
});

it.each(["replacement", "A-B-A"])(
  "rejects a saved snapshot from a retired %s owner",
  async (retirement) => {
    const view = render(
      <Owner>
        <Mobile />
      </Owner>,
    );
    act(() => mobile.handleOpenFile(file));
    act(() => mobile.handleSelectedFileChange("B"));
    view.rerender(
      <Owner>
        <Desktop />
      </Owner>,
    );
    const reply = deferred();
    mocks.save.mockReturnValue(reply.promise);
    let pending!: Promise<void>;
    act(() => {
      pending = desktop.saveFile(file.path);
    });
    if (retirement === "replacement")
      act(() =>
        useDockviewStore.getState().setFileState(file.path, { ...file, content: "replacement" }),
      );
    else {
      act(() =>
        app.setState((state) => ({ tasks: { ...state.tasks, activeSessionId: "draft-B" } })),
      );
      act(() =>
        app.setState((state) => ({ tasks: { ...state.tasks, activeSessionId: "draft-A" } })),
      );
    }
    const replacement = current();
    await act(async () => {
      reply.resolve({ success: true, new_hash: "stale-hash" });
      await pending;
    });
    expect(current()).toBe(replacement);
    expect(current()?.originalHash).toBe("hash-A");
  },
);

// @covers AC-UI-MARKDOWN-FILE-EDITING-003.9
it("retains draft and destination when routing is cancelled and registers unload protection", () => {
  render(
    <Owner>
      <Mobile />
    </Owner>,
  );
  act(() => mobile.handleOpenFile(file));
  act(() => mobile.handleSelectedFileChange("B"));
  const navigate = vi.fn();
  act(() => requestNavigation(navigate));
  expect(mocks.dialog?.open).toBe(true);
  act(() => mocks.dialog?.onCancel());
  expect(navigate).not.toHaveBeenCalled();
  expect(current()?.content).toBe("B");
  const unload = new Event("beforeunload", { cancelable: true });
  window.dispatchEvent(unload);
  expect(unload.defaultPrevented).toBe(true);
  act(() => requestNavigation(navigate));
  act(() => mocks.dialog?.onConfirm());
  expect(navigate).toHaveBeenCalledOnce();
  expect(current()).toBeUndefined();
  const cleanUnload = new Event("beforeunload", { cancelable: true });
  window.dispatchEvent(cleanUnload);
  expect(cleanUnload.defaultPrevented).toBe(false);
});

// @covers AC-UI-MARKDOWN-FILE-EDITING-002.14
it("accepts the remote draft baseline across rotation and saves the next edit against it", async () => {
  const view = render(
    <Owner>
      <Mobile />
    </Owner>,
  );
  act(() =>
    app.setState((state) => ({ connection: { ...state.connection, status: "connected" } })),
  );
  act(() => mobile.handleOpenFile(file));
  act(() => mobile.handleSelectedFileChange("B"));
  let resolve!: (value: { content: string }) => void;
  mocks.read.mockReturnValue(
    new Promise((yes) => {
      resolve = yes;
    }),
  );
  act(() =>
    mocks.client.on.mock.calls.at(-1)?.[1]({
      payload: {
        session_id: "draft-A",
        changes: [{ path: file.path, operation: "modify" }],
      },
    }),
  );
  view.rerender(
    <Owner>
      <Desktop />
    </Owner>,
  );
  await act(async () => {
    resolve({ content: "B" });
  });
  await waitFor(() =>
    expect(current()).toMatchObject({ content: "B", originalContent: "B", isDirty: false }),
  );
  const acceptedHash = current()?.originalHash;
  expect(acceptedHash).not.toBe("hash-A");
  act(() => desktop.handleFileChange(file.path, "C"));
  mocks.save.mockResolvedValue({ success: true, new_hash: "hash-C" });
  await act(async () => desktop.saveFile(file.path));
  expect(mocks.save).toHaveBeenCalledWith(
    mocks.client,
    "draft-A",
    expect.objectContaining({ originalHash: acceptedHash, desiredContent: "C" }),
  );
});

it("does not discard a replacement draft through an old navigation decision", () => {
  render(
    <Owner>
      <Mobile />
    </Owner>,
  );
  act(() => mobile.handleOpenFile(file));
  act(() => mobile.handleSelectedFileChange("B"));
  const navigate = vi.fn();
  act(() => requestNavigation(navigate));
  act(() =>
    useDockviewStore
      .getState()
      .setFileState(file.path, { ...file, content: "replacement", isDirty: true }),
  );
  act(() => mocks.dialog?.onConfirm());
  expect(navigate).not.toHaveBeenCalled();
  expect(current()?.content).toBe("replacement");
});

// @covers AC-UI-MARKDOWN-FILE-EDITING-003.8
it("edits the same canonical draft on tablet and acknowledges its save after rotating to phone", async () => {
  const view = render(
    <Owner>
      <Mobile />
    </Owner>,
  );
  act(() => mobile.handleOpenFile(file));
  act(() => mobile.handleSelectedFileChange("B"));
  const instance = current()?.instanceId;
  view.rerender(
    <Owner>
      <Tablet />
    </Owner>,
  );
  expect(mocks.tablet?.tab.content).toBe("B");
  expect(mocks.tablet?.tab).toBe(current());
  act(() => mocks.tablet?.onFileChange(file.path, "C"));
  const reply = deferred();
  mocks.save.mockReturnValue(reply.promise);
  let pending!: Promise<void>;
  act(() => {
    pending = mocks.tablet!.onFileSave(file.path);
  });
  view.rerender(
    <Owner>
      <Mobile />
    </Owner>,
  );
  expect(mobile.selectedFile).toMatchObject({ content: "C", instanceId: instance });
  act(() => mobile.handleSelectedFileChange("D"));
  await act(async () => {
    reply.resolve({ success: true, new_hash: "hash-C" });
    await pending;
  });
  expect(current()).toMatchObject({
    content: "D",
    originalContent: "C",
    originalHash: "hash-C",
    isDirty: true,
  });
});

it.each(["replacement", "A-B-A"])(
  "rejects an outside read after %s changes its canonical owner",
  async (retirement) => {
    const sync = vi.spyOn(workspaceSync, "syncOpenFileFromWorkspace");
    render(
      <Owner>
        <Mobile />
      </Owner>,
    );
    act(() =>
      app.setState((state) => ({ connection: { ...state.connection, status: "connected" } })),
    );
    act(() => mobile.handleOpenFile(file));
    let resolve!: (value: { content: string }) => void;
    let resolveActivation!: (value: { content: string }) => void;
    const activation = new Promise((yes) => {
      resolveActivation = yes;
    });
    mocks.read
      .mockReturnValueOnce(
        new Promise((yes) => {
          resolve = yes;
        }),
      )
      .mockReturnValue(activation);
    act(() =>
      mocks.client.on.mock.calls.at(-1)?.[1]({
        payload: {
          session_id: "draft-A",
          changes: [{ path: file.path, operation: "modify" }],
        },
      }),
    );
    const oldRead = sync.mock.results.at(-1)?.value as Promise<void>;
    if (retirement === "replacement")
      act(() =>
        useDockviewStore.getState().setFileState(file.path, { ...file, content: "replacement" }),
      );
    else {
      act(() =>
        app.setState((state) => ({ tasks: { ...state.tasks, activeSessionId: "draft-B" } })),
      );
      act(() =>
        app.setState((state) => ({ tasks: { ...state.tasks, activeSessionId: "draft-A" } })),
      );
    }
    const replacement = current();
    await act(async () => {
      resolve({ content: "obsolete outside read" });
      await oldRead;
    });
    expect(current()).toBe(replacement);
    await act(async () => {
      resolveActivation({ content: "A" });
      await Promise.all(sync.mock.results.map((result) => result.value));
    });
    // Observe a completed positive control before asserting that the old request stayed retired.
    mocks.read.mockResolvedValue({ content: "current outside read" });
    act(() =>
      mocks.client.on.mock.calls.at(-1)?.[1]({
        payload: {
          session_id: "draft-A",
          changes: [{ path: file.path, operation: "modify" }],
        },
      }),
    );
    await waitFor(() => expect(current()?.content).toBe("current outside read"));
    expect(replacement?.originalContent).toBe("A");
    expect(current()?.content).not.toBe("obsolete outside read");
  },
);

it("finishes restoration after a same-instance presentation change", async () => {
  setOpenFileTabs("restore-presentation", [
    { path: file.path, name: file.name, markdownMode: "edit" },
  ]);
  let resolve!: (response: { content: string }) => void;
  mocks.read.mockReturnValue(
    new Promise((yes) => {
      resolve = yes;
    }),
  );
  render(
    <Owner initialSessionId="restore-presentation">
      <Desktop />
    </Owner>,
  );
  act(() => useDockviewStore.getState().updateFileState(file.path, { markdownMode: "source" }));
  await act(async () => resolve({ content: "restored bytes" }));
  await waitFor(() => expect(current()?.content).toBe("restored bytes"));
  expect(current()?.markdownMode).toBe("source");
});

// @covers AC-UI-MARKDOWN-FILE-EDITING-002.4, AC-UI-MARKDOWN-FILE-EDITING-003.8
it.each(["session activation", "reconnection"])(
  "refreshes retained clean drafts on %s after missing outside changes",
  async (trigger) => {
    const view = render(
      <Owner>
        <Mobile />
      </Owner>,
    );
    if (trigger === "reconnection")
      act(() =>
        app.setState((state) => ({ connection: { ...state.connection, status: "connected" } })),
      );
    act(() => mobile.handleOpenFile(file));
    const instance = current()?.instanceId;
    view.rerender(
      <Owner>
        <Desktop />
      </Owner>,
    );
    if (trigger === "session activation") {
      act(() =>
        app.setState((state) => ({ tasks: { ...state.tasks, activeSessionId: "draft-B" } })),
      );
      act(() =>
        app.setState((state) => ({ connection: { ...state.connection, status: "connected" } })),
      );
    } else
      act(() =>
        app.setState((state) => ({ connection: { ...state.connection, status: "disconnected" } })),
      );
    mocks.read.mockResolvedValue({ content: "outside edit while absent" });
    if (trigger === "session activation")
      act(() =>
        app.setState((state) => ({ tasks: { ...state.tasks, activeSessionId: "draft-A" } })),
      );
    else
      act(() =>
        app.setState((state) => ({ connection: { ...state.connection, status: "connected" } })),
      );
    await waitFor(() =>
      expect(current()).toMatchObject({
        content: "outside edit while absent",
        originalContent: "outside edit while absent",
        isDirty: false,
        instanceId: instance,
      }),
    );
    expect(current()?.originalHash).not.toBe("hash-A");
    expect(mocks.read).toHaveBeenCalledWith(mocks.client, "draft-A", file.path, undefined);
  },
);

it("revalidates dirty drafts on reconnect without replacing unsaved bytes", async () => {
  render(
    <Owner>
      <Mobile />
    </Owner>,
  );
  act(() =>
    app.setState((state) => ({ connection: { ...state.connection, status: "connected" } })),
  );
  act(() => mobile.handleOpenFile(file));
  act(() => mobile.handleSelectedFileChange("local draft"));
  act(() =>
    app.setState((state) => ({ connection: { ...state.connection, status: "disconnected" } })),
  );
  mocks.read.mockResolvedValue({ content: "outside change" });
  act(() =>
    app.setState((state) => ({ connection: { ...state.connection, status: "connected" } })),
  );
  await waitFor(() =>
    expect(current()).toMatchObject({
      content: "local draft",
      originalContent: "A",
      isDirty: true,
      hasRemoteUpdate: true,
      remoteContent: "outside change",
    }),
  );
});

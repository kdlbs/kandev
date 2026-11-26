import { useState } from "react";
import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useTabletSessionTab } from "@/hooks/domains/session/use-tablet-session-tab";
import {
  getActiveTabForSession,
  getOpenFileTabs,
  setActiveTabForSession,
  setOpenFileTabs,
} from "@/lib/local-storage";
import type { OpenFileTab } from "@/lib/types/backend";
import { getFileTabKey } from "./task-center-panel-file-tabs";
import { useFileBrowserOpen } from "./use-file-browser-open";
import { TaskCenterPanel } from "./task-center-panel";
import { useFilesPanelData } from "./task-files-panel-hooks";
import { createAppStore } from "@/lib/state/store";
import {
  bindTabletFileOpen,
  captureTabletFileOpen,
  captureTabletFileDeletion,
} from "@/lib/state/tablet-file-authority";
import { useTabletFileRequest } from "./task-center-panel-state";

const OPEN_TREE_FILE = "Open tree file";
const CHAT_TAB = "chat";
const data = vi.hoisted(() => ({
  client: {},
  read: vi.fn(),
  remove: vi.fn(),
  create: vi.fn(),
  toast: vi.fn(),
  git: undefined as unknown,
  commits: [] as unknown[],
  loaded: false,
  loading: true,
  open: null as null | ((path: string, repo?: string) => Promise<void>),
  files: new Map<
    string,
    { file: OpenFileTab; onFileChange: (path: string, content: string, repo?: string) => void }
  >(),
}));
vi.mock("@/lib/ws/connection", async (original) => ({
  ...(await original<object>()),
  getWebSocketClient: () => data.client,
}));
vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileContent: (...args: unknown[]) => data.read(...args),
  deleteFile: (...args: unknown[]) => data.remove(...args),
  createFile: (...args: unknown[]) => data.create(...args),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: data.toast }) }));
vi.mock("@/hooks/domains/session/use-session-git-status", async (original) => ({
  ...(await original<object>()),
  useSessionGitStatus: () => data.git,
}));
vi.mock("@/hooks/domains/session/use-session-commits", () => ({
  useSessionCommits: () => ({
    commits: data.commits,
    loaded: data.loaded,
    loading: data.loading,
  }),
}));
vi.mock("./task-chat-panel", () => ({
  TaskChatPanel: (props: { onOpenFile?: typeof data.open }) => {
    data.open = props.onOpenFile ?? null;
    return null;
  },
}));
vi.mock("./task-changes-panel", () => ({ TaskChangesPanel: () => null }));
vi.mock("./file-tab-content", () => ({
  FileTabContent: (props: {
    tab: OpenFileTab;
    onFileChange: (path: string, content: string, repo?: string) => void;
  }) => {
    data.files.set(getFileTabKey(props.tab), { file: props.tab, onFileChange: props.onFileChange });
    return null;
  },
}));
let store: ReturnType<typeof useAppStoreApi>;
let choose!: (tab: string) => void;
let treeOpening: Promise<void> | undefined;
let openPath = "F.txt";
let removeFile!: (path: string) => Promise<boolean>;
let createFile!: (path: string) => Promise<boolean>;
function Probe() {
  store = useAppStoreApi();
  const [, select] = useTabletSessionTab("B");
  choose = select;
  const [request, setRequest] = useState<OpenFileTab | null>(null);
  const open = useFileBrowserOpen("B", setRequest);
  const operations = useFilesPanelData(setRequest);
  removeFile = operations.hookDeleteFile;
  createFile = operations.handleCreateFile;
  return (
    <>
      <button
        onClick={() => {
          treeOpening = open(openPath);
        }}
      >
        {OPEN_TREE_FILE}
      </button>
      <TaskCenterPanel
        sessionId="B"
        selectedDiff={null}
        openFileRequest={request}
        onFileOpenHandled={() => setRequest(null)}
        onDiffHandled={() => {}}
      />
    </>
  );
}
function setup() {
  const view = render(
    <StateProvider>
      <TooltipProvider>
        <Probe />
      </TooltipProvider>
    </StateProvider>,
  );
  act(() =>
    store.setState({
      tasks: { ...store.getState().tasks, activeTaskId: "T", activeSessionId: "B" },
    }),
  );
  return view;
}
function readyGit(files: Record<string, unknown>) {
  data.git = { status_state: "ready", files_complete: true, files };
  data.loaded = true;
  data.loading = false;
  act(() => {
    store.getState().reconcileGitStatusInventory("B", 0, [""]);
  });
}
afterEach(() => {
  cleanup();
  sessionStorage.clear();
  data.read.mockReset();
  data.remove.mockReset();
  data.create.mockReset();
  openPath = "F.txt";
  data.toast.mockClear();
  data.git = undefined;
  data.commits = [];
  data.loaded = false;
  data.loading = true;
  data.open = null;
  data.files.clear();
});
it("falls back from a saved PR tab to Chat when the task has no linked reviews", async () => {
  setActiveTabForSession("B", "pr");
  setup();

  await waitFor(() => {
    expect(store.getState().tabletSession.activeTabBySessionId.B).toBe(CHAT_TAB);
  });
  expect(getActiveTabForSession("B", CHAT_TAB)).toBe(CHAT_TAB);
  expect(
    screen.getByRole("tab", { name: new RegExp(CHAT_TAB, "i") }).getAttribute("aria-selected"),
  ).toBe("true");
});

it("does not let an older Files-tree reply replace newer tablet Chat", async () => {
  const reply = Promise.withResolvers<{ content: string }>();
  data.read.mockReturnValue(reply.promise);
  setup();
  fireEvent.click(screen.getByRole("button", { name: OPEN_TREE_FILE }));
  act(() => choose(CHAT_TAB));
  await act(async () => {
    reply.resolve({ content: "old tree reply" });
    await treeOpening;
  });
  expect(store.getState().tabletSession.activeTabBySessionId.B).toBe(CHAT_TAB);
  expect(getOpenFileTabs("B")).toEqual([]);
  data.read.mockResolvedValue({ content: "fresh tree reply" });
  fireEvent.click(screen.getByRole("button", { name: OPEN_TREE_FILE }));
  await waitFor(() =>
    expect(store.getState().tabletSession.activeTabBySessionId.B).toBe(
      `file:${getFileTabKey({ path: "F.txt" })}`,
    ),
  );
});
it("selects the current repo alias and preserves its dirty buffer when a winning open completes", async () => {
  setOpenFileTabs("B", [{ path: "F.txt", repo: "repo", name: "F.txt" }]);
  const restored = Promise.withResolvers<{ content: string }>();
  const opened = Promise.withResolvers<{ content: string }>();
  data.read.mockImplementation((_client, _session, path) =>
    path === "repo/F.txt" ? opened.promise : restored.promise,
  );
  setup();
  let opening!: Promise<void>;
  act(() => {
    opening = data.open!("repo/F.txt");
  });
  await act(async () => restored.resolve({ content: "saved disk" }));
  const key = getFileTabKey({ path: "F.txt", repo: "repo" });
  await waitFor(() => expect(data.files.has(key)).toBe(true));
  act(() => data.files.get(key)!.onFileChange("F.txt", "unsaved draft", "repo"));
  await act(async () => {
    opened.resolve({ content: "new disk" });
    await opening;
  });
  expect(store.getState().tabletSession.activeTabBySessionId.B).toBe(`file:${key}`);
  expect(getActiveTabForSession("B", CHAT_TAB)).toBe(`file:${key}`);
  expect(data.files.get(key)!.file).toMatchObject({ content: "unsaved draft", isDirty: true });
  expect(screen.getByRole("tab", { name: /F.txt/ }).getAttribute("data-state")).toBe("active");
});
it("keeps initial Changes while Git evidence is pending, then accepts nonempty evidence", async () => {
  setActiveTabForSession("B", "changes");
  const view = setup();
  await act(async () => {});
  expect(store.getState().tabletSession.activeTabBySessionId.B).toBe("changes");
  readyGit({ "F.txt": {} });
  view.rerender(
    <StateProvider>
      <TooltipProvider>
        <Probe />
      </TooltipProvider>
    </StateProvider>,
  );
  expect(getActiveTabForSession("B", CHAT_TAB)).toBe("changes");
  expect(screen.getByRole("tab", { name: /All changes/i }).getAttribute("data-state")).toBe(
    "active",
  );
});
it("falls back only after ready-empty Git evidence and preserves newer explicit Chat", async () => {
  setActiveTabForSession("B", "changes");
  const view = setup();
  readyGit({});
  view.rerender(
    <StateProvider>
      <TooltipProvider>
        <Probe />
      </TooltipProvider>
    </StateProvider>,
  );
  await waitFor(() => expect(getActiveTabForSession("B", "changes")).toBe(CHAT_TAB));
  act(() => choose(CHAT_TAB));
  readyGit({ "F.txt": {} });
  view.rerender(
    <StateProvider>
      <TooltipProvider>
        <Probe />
      </TooltipProvider>
    </StateProvider>,
  );
  expect(store.getState().tabletSession.activeTabBySessionId.B).toBe(CHAT_TAB);
});

it.each(
  ["tree", "transcript"].flatMap((producer) =>
    [false, true].flatMap((parent) =>
      [false, true].map((success) => ({ producer, parent, success })),
    ),
  ),
)(
  "fences deleted pending $producer content (parent=$parent, success=$success)",
  async ({ producer, parent, success }) => {
    const reply = Promise.withResolvers<{ content: string }>();
    data.read.mockReturnValue(reply.promise);
    data.remove.mockResolvedValue({ success });
    openPath = "dir/F.txt";
    setup();
    let opening: Promise<void> | undefined;
    act(() => {
      if (producer === "tree") {
        fireEvent.click(screen.getByRole("button", { name: OPEN_TREE_FILE }));
        opening = treeOpening;
      } else opening = data.open!(openPath);
    });
    await act(async () => {
      await removeFile(parent ? "dir" : openPath);
    });
    await act(async () => {
      reply.resolve({ content: "deleted bytes" });
      await opening;
    });
    const fileKey = `file:${getFileTabKey({ path: openPath })}`;
    expect(store.getState().tabletSession.activeTabBySessionId.B).toBe(
      success ? CHAT_TAB : fileKey,
    );
    expect(getOpenFileTabs("B").map((tab) => tab.path)).toEqual(success ? [] : [openPath]);
    data.read.mockResolvedValue({ content: "recreated file" });
    await act(async () => {
      await data.open!(openPath);
    });
    expect(store.getState().tabletSession.activeTabBySessionId.B).toBe(fileKey);
  },
);

it("keeps the reopened dirty editor when another path is deleted", async () => {
  data.read.mockResolvedValue({ content: "disk F" });
  data.remove.mockResolvedValue({ success: true });
  setup();
  fireEvent.click(screen.getByRole("button", { name: OPEN_TREE_FILE }));
  await act(async () => {
    await treeOpening;
  });
  fireEvent.click(within(screen.getByRole("tab", { name: "F.txt" })).getByRole("button"));
  await act(async () => {
    await data.open!("F.txt");
  });
  const key = getFileTabKey({ path: "F.txt" });
  act(() => data.files.get(key)!.onFileChange("F.txt", "reopened dirty buffer"));
  await act(async () => {
    await removeFile("G.txt");
  });
  expect(store.getState().tabletSession.activeTabBySessionId.B).toBe(`file:${key}`);
  expect(getOpenFileTabs("B").map((tab) => tab.path)).toEqual(["F.txt"]);
  expect(data.files.get(key)!.file).toMatchObject({
    content: "reopened dirty buffer",
    isDirty: true,
  });
  expect(screen.getByRole("tab", { name: /F.txt/ }).getAttribute("data-state")).toBe("active");
});

it.each([false, true])(
  "does not let New File replace newer Chat (superseded=%s)",
  async (superseded) => {
    const reply = Promise.withResolvers<{ success: boolean }>();
    data.create.mockReturnValue(reply.promise);
    setup();
    let creating!: Promise<boolean>;
    act(() => {
      creating = createFile("new.txt");
    });
    if (superseded) act(() => choose(CHAT_TAB));
    await act(async () => {
      reply.resolve({ success: true });
      expect(await creating).toBe(true);
    });
    expect(store.getState().tabletSession.activeTabBySessionId.B).toBe(
      superseded ? CHAT_TAB : `file:${getFileTabKey({ path: "new.txt" })}`,
    );
    expect(getOpenFileTabs("B").map((tab) => tab.path)).toEqual(superseded ? [] : ["new.txt"]);
  },
);

it.each([false, true])(
  "lets a pending file intent settle before empty Changes (success=%s)",
  async (success) => {
    setActiveTabForSession("B", "changes");
    const reply = Promise.withResolvers<{ content: string }>();
    data.read.mockReturnValue(reply.promise);
    const view = setup();
    fireEvent.click(screen.getByRole("button", { name: OPEN_TREE_FILE }));
    readyGit({});
    view.rerender(
      <StateProvider>
        <TooltipProvider>
          <Probe />
        </TooltipProvider>
      </StateProvider>,
    );
    try {
      await act(async () => {});
      expect(store.getState().tabletSession.activeTabBySessionId.B).toBe("changes");
      await act(async () => {
        if (success) reply.resolve({ content: "F" });
        else reply.reject(new Error("get failed"));
        await treeOpening;
      });
      await waitFor(() =>
        expect(store.getState().tabletSession.activeTabBySessionId.B).toBe(
          success ? `file:${getFileTabKey({ path: "F.txt" })}` : CHAT_TAB,
        ),
      );
    } finally {
      reply.resolve({ content: "cleanup" });
    }
  },
);

it.each([false, true])(
  "rechecks a delivered file at consumer publication (deleted=%s)",
  async (deleted) => {
    const app = createAppStore();
    app.getState().setTabletSessionTab("B", CHAT_TAB);
    const file: OpenFileTab = {
      path: "F.txt",
      name: "F.txt",
      content: "F",
      originalContent: "F",
      originalHash: "hash",
      isDirty: false,
    };
    const operation = captureTabletFileOpen(app, "B", "F.txt", () => true);
    bindTabletFileOpen(file, operation);
    let acknowledged = 0;
    renderHook(() =>
      useTabletFileRequest(
        file,
        () => {
          acknowledged++;
        },
        (accepted) => {
          app.getState().setTabletSessionTab("B", `file:${getFileTabKey(accepted)}`);
        },
      ),
    );
    if (deleted) captureTabletFileDeletion(app, "B", "F.txt")();
    await act(async () => {});
    expect(app.getState().tabletSession.activeTabBySessionId.B).toBe(
      deleted ? CHAT_TAB : `file:${getFileTabKey(file)}`,
    );
    expect(acknowledged).toBe(1);
  },
);

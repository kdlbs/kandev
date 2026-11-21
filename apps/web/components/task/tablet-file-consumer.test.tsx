import { useState } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
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

const data = vi.hoisted(() => ({
  client: {},
  read: vi.fn(),
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
function Probe() {
  store = useAppStoreApi();
  const [, select] = useTabletSessionTab("B");
  choose = select;
  const [request, setRequest] = useState<OpenFileTab | null>(null);
  const open = useFileBrowserOpen("B", setRequest);
  return (
    <>
      <button
        onClick={() => {
          treeOpening = open("F.txt");
        }}
      >
        Open tree file
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
afterEach(() => {
  cleanup();
  sessionStorage.clear();
  data.read.mockReset();
  data.toast.mockClear();
  data.git = undefined;
  data.commits = [];
  data.loaded = false;
  data.loading = true;
  data.open = null;
  data.files.clear();
});
it("does not let an older Files-tree reply replace newer tablet Chat", async () => {
  const reply = Promise.withResolvers<{ content: string }>();
  data.read.mockReturnValue(reply.promise);
  setup();
  fireEvent.click(screen.getByRole("button", { name: "Open tree file" }));
  act(() => choose("chat"));
  await act(async () => {
    reply.resolve({ content: "old tree reply" });
    await treeOpening;
  });
  expect(store.getState().tabletSession.activeTabBySessionId.B).toBe("chat");
  expect(getOpenFileTabs("B")).toEqual([]);
  data.read.mockResolvedValue({ content: "fresh tree reply" });
  fireEvent.click(screen.getByRole("button", { name: "Open tree file" }));
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
  expect(getActiveTabForSession("B", "chat")).toBe(`file:${key}`);
  expect(data.files.get(key)!.file).toMatchObject({ content: "unsaved draft", isDirty: true });
  expect(screen.getByRole("tab", { name: /F.txt/ }).getAttribute("data-state")).toBe("active");
});
it("keeps initial Changes while Git evidence is pending, then accepts nonempty evidence", async () => {
  setActiveTabForSession("B", "changes");
  const view = setup();
  await act(async () => {});
  expect(store.getState().tabletSession.activeTabBySessionId.B).toBe("changes");
  data.git = { status_state: "ready", files_complete: true, files: { "F.txt": {} } };
  data.loaded = true;
  data.loading = false;
  view.rerender(
    <StateProvider>
      <TooltipProvider>
        <Probe />
      </TooltipProvider>
    </StateProvider>,
  );
  expect(getActiveTabForSession("B", "chat")).toBe("changes");
  expect(screen.getByRole("tab", { name: /All changes/i }).getAttribute("data-state")).toBe(
    "active",
  );
});
it("falls back only after ready-empty Git evidence and preserves newer explicit Chat", async () => {
  setActiveTabForSession("B", "changes");
  const view = setup();
  data.git = { status_state: "ready", files_complete: true, files: {} };
  data.loaded = true;
  data.loading = false;
  view.rerender(
    <StateProvider>
      <TooltipProvider>
        <Probe />
      </TooltipProvider>
    </StateProvider>,
  );
  await waitFor(() => expect(getActiveTabForSession("B", "changes")).toBe("chat"));
  act(() => choose("chat"));
  data.git = { status_state: "ready", files_complete: true, files: { "F.txt": {} } };
  view.rerender(
    <StateProvider>
      <TooltipProvider>
        <Probe />
      </TooltipProvider>
    </StateProvider>,
  );
  expect(store.getState().tabletSession.activeTabBySessionId.B).toBe("chat");
});

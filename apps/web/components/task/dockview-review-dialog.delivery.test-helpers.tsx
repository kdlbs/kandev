import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { expect, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { VcsDialogsProvider } from "@/components/vcs/vcs-dialogs";
import { WebSocketClient } from "@/lib/ws/client";
import { getWebSocketClient, setWebSocketClient } from "@/lib/ws/connection";
import {
  useCommentsStore,
  loadSessionComments,
  type Comment,
  type ReviewComment,
} from "@/lib/state/slices/comments";
import type { StoreProviderProps, AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import { TaskReviewDialogMount } from "./dockview-review-dialog";
import { SessionMobileReviewDialog } from "./mobile/session-mobile-review-dialog";

type Request = { id: string; type: string; action: string; payload: Record<string, unknown> };
type Scope = { taskId: string; sessionId: string };
export type Surface = "desktop" | "phone";
const FILE_PATH = "handlers/save.ts";
const REPO_CORE = "core";
const REPO_UI = "ui";
const unexpected: string[] = [];
let sequence = 0;
let store: StoreApi<AppState>;
let originalClient: WebSocketClient | null;
let client: WebSocketClient;
let animations: PropertyDescriptor | undefined;

class ReviewSocket {
  static readonly OPEN = 1;
  static current: ReviewSocket;
  readyState = 0;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  requests: Request[] = [];
  settled = new Set<string>();

  constructor() {
    ReviewSocket.current = this;
  }
  close() {
    this.readyState = 3;
  }
  send(data: string) {
    const request = JSON.parse(data) as Request;
    this.requests.push(request);
    if (request.action === "message.add") return;
    const responses: Record<string, unknown> = {
      "session.file_review.get": { reviews: [] },
      "session.cumulative_diff": { cumulative_diff: null, ready: true },
    };
    if (!(request.action in responses)) unexpected.push(request.action);
    queueMicrotask(() => this.reply(request, responses[request.action] ?? {}));
  }
  reply(request: Request, payload: unknown, type = "response") {
    expect(this.settled.has(request.id)).toBe(false);
    this.settled.add(request.id);
    this.onmessage?.({
      data: JSON.stringify({ id: request.id, type, action: request.action, payload }),
    });
  }
}

function CaptureStore() {
  store = useAppStoreApi();
  return null;
}

function gitStatus(repositoryName: string): GitStatusEntry {
  return {
    branch: "review",
    remote_branch: null,
    modified: [FILE_PATH],
    added: [],
    deleted: [],
    untracked: [],
    renamed: [],
    ahead: 0,
    behind: 0,
    timestamp: null,
    repository_name: repositoryName,
    status_state: "ready",
    detail_state: "ready",
    files_complete: true,
    files: {
      [FILE_PATH]: {
        path: FILE_PATH,
        status: "modified",
        staged: false,
        additions: 1,
        deletions: 1,
        diff: "@@ -1 +1 @@\n-before\n+after",
        diff_state: "ready",
      },
    },
  };
}

function MountedReview({
  surface,
  scope,
  state,
}: {
  surface: Surface;
  scope: Scope;
  state: StoreProviderProps["initialState"];
}) {
  return (
    <StateProvider initialState={state}>
      <CaptureStore />
      <ToastProvider>
        <TooltipProvider>
          <VcsDialogsProvider sessionId={scope.sessionId}>
            {surface === "phone" ? (
              <SessionMobileReviewDialog {...scope} onOpenFile={() => {}} />
            ) : (
              <TaskReviewDialogMount {...scope} />
            )}
          </VcsDialogsProvider>
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>
  );
}

export function prepareDeliveryFixture(surface: Surface) {
  localStorage.clear();
  sessionStorage.clear();
  unexpected.length = 0;
  useCommentsStore.setState({
    byId: {},
    bySession: {},
    pendingForChat: [],
    editingCommentId: null,
  });
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "queueMicrotask"] });
  originalClient = getWebSocketClient();
  vi.stubGlobal("WebSocket", ReviewSocket);
  vi.spyOn(window, "matchMedia").mockImplementation((query) => ({
    matches: query.includes("pointer: fine") ? surface === "desktop" : surface === "phone",
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => true,
  }));
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL) => {
      if (String(input) === "http://localhost:3000/api/v1/system/logs/frontend-errors") {
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      unexpected.push(`fetch:${String(input)}`);
      return Promise.resolve(new Response("{}", { status: 500 }));
    }),
  );
  animations = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "getAnimations");
  if (!animations)
    Object.defineProperty(HTMLElement.prototype, "getAnimations", {
      configurable: true,
      value: () => [],
    });
  client = new WebSocketClient("ws://review-delivery-fixture", undefined, { enabled: false });
  client.connect();
  ReviewSocket.current.readyState = ReviewSocket.OPEN;
  ReviewSocket.current.onopen?.();
  setWebSocketClient(client);
}

export async function disposeDeliveryFixture() {
  await act(async () => {
    for (const request of ReviewSocket.current.requests) {
      if (!ReviewSocket.current.settled.has(request.id)) ReviewSocket.current.reply(request, {});
    }
    vi.runAllTicks();
  });
  cleanup();
  client.disconnect();
  setWebSocketClient(originalClient);
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  if (animations) Object.defineProperty(HTMLElement.prototype, "getAnimations", animations);
  else Reflect.deleteProperty(HTMLElement.prototype, "getAnimations");
  useCommentsStore.setState({
    byId: {},
    bySession: {},
    pendingForChat: [],
    editingCommentId: null,
  });
  sessionStorage.clear();
  localStorage.clear();
  expect(unexpected).toEqual([]);
}

export async function flushDelivery() {
  await act(async () => {
    vi.runAllTicks();
    await vi.advanceTimersByTimeAsync(0);
  });
}

export function commentsFor(sessionId: string): ReviewComment[] {
  return [
    {
      id: `line-${sessionId}`,
      sessionId,
      source: "diff",
      status: "pending",
      filePath: FILE_PATH,
      repositoryName: REPO_CORE,
      repositoryId: "repo-core",
      startLine: 8,
      endLine: 9,
      side: "deletions",
      codeContent: "await save();\nreturn draft;",
      // i18n-exempt: Synthetic authored feedback testing exact raw text, not product copy.
      text: "  Keep this draft.\nKeep the retry too.  ",
      createdAt: "2026-10-08T10:00:00Z",
    },
    {
      id: `file-${sessionId}`,
      sessionId,
      source: "review-file",
      status: "pending",
      filePath: FILE_PATH,
      repositoryName: REPO_UI,
      repositoryId: "repo-ui",
      baseRef: "parent-gitlink",
      isSubmodule: true,
      // i18n-exempt: Synthetic authored feedback testing exact raw text, not product copy.
      text: "Wait for acknowledgement.\nDo not erase edits.",
      createdAt: "2026-10-08T10:00:01Z",
    },
  ];
}

export const submittedMarkdown = [
  "### Review Comments",
  "",
  "**core/handlers/save.ts:8-9**",
  "```",
  "await save();",
  "return draft;",
  "```",
  ">   Keep this draft.\nKeep the retry too.  ",
  "",
  "**ui/handlers/save.ts**",
  "> Wait for acknowledgement.",
  "> Do not erase edits.",
  "",
  "---",
  "",
].join("\n");

export async function openDelivery(surface: Surface) {
  const primary = {
    taskId: `delivery-task-${++sequence}`,
    sessionId: `delivery-session-${sequence}`,
  };
  const other = { taskId: `other-task-${sequence}`, sessionId: `other-session-${sequence}` };
  const notes = commentsFor(primary.sessionId);
  notes.forEach((note) => useCommentsStore.getState().addComment(note));
  const state = {};
  const mounted = render(<MountedReview surface={surface} scope={primary} state={state} />);
  act(() => {
    store.getState().setActiveSession(primary.taskId, primary.sessionId);
    store
      .getState()
      .setUserSettings({ ...store.getState().userSettings, reviewAutoMarkOnScroll: false });
    for (const scope of [primary, other]) {
      store.getState().registerSessionEnvironment(scope.sessionId, `env-${scope.sessionId}`);
      store.getState().setGitStatus(`env-${scope.sessionId}`, gitStatus(REPO_CORE));
      store.getState().setGitStatus(`env-${scope.sessionId}`, gitStatus(REPO_UI));
      store.getState().setTaskReview(scope.taskId, { runs: [], findings: [] });
    }
  });
  await flushDelivery();
  await reopenDelivery();
  return {
    primary,
    other,
    notes,
    switchToOther: async () => {
      act(() => store.getState().setActiveSession(other.taskId, other.sessionId));
      mounted.rerender(<MountedReview surface={surface} scope={other} state={state} />);
      await flushDelivery();
    },
  };
}

export async function reopenDelivery() {
  act(() => window.dispatchEvent(new Event("open-review-dialog")));
  await flushDelivery();
  expect(screen.getByRole("dialog", { name: "Review Changes" })).toBeDefined();
}

export function deliveryButton() {
  return within(screen.getByRole("dialog", { name: "Review Changes" })).getByTestId(
    "review-fix-comments-button",
  );
}

export function clickDelivery(surface: Surface) {
  const before = sends().length;
  const overviewOpen = screen.queryByTestId("review-comments-overview") !== null;
  fireEvent.click(deliveryButton());
  if (surface === "phone" && !overviewOpen) {
    expect(sends()).toHaveLength(before);
    expect(screen.getByTestId("review-comments-overview")).toBeDefined();
    fireEvent.click(deliveryButton());
  }
}

export function sends() {
  return ReviewSocket.current.requests.filter((r) => r.action === "message.add");
}

export async function acknowledge(index = 0, error?: string) {
  await act(async () => {
    ReviewSocket.current.reply(
      sends()[index],
      error ? { message: error, code: "send_rejected" } : {},
      error ? "error" : "response",
    );
    vi.runAllTicks();
  });
}

export function assertRetained(notes: Comment[], sessionId: string) {
  expect
    .soft(
      useCommentsStore
        .getState()
        .getPendingComments()
        .filter((c) => c.sessionId === sessionId),
    )
    .toEqual(notes);
  expect.soft(loadSessionComments(sessionId)).toEqual(notes);
  expect
    .soft(useCommentsStore.getState().bySession[sessionId] ?? [])
    .toEqual(notes.map((n) => n.id));
  for (const note of notes) expect.soft(useCommentsStore.getState().byId[note.id]).toEqual(note);
}

export function disconnectDelivery() {
  act(() => setWebSocketClient(null));
}
export function reconnectDelivery() {
  act(() => setWebSocketClient(client));
}
export function closeDelivery() {
  fireEvent.click(screen.getByRole("button", { name: "Close review" }));
}
export function isReviewOpen() {
  return screen.queryByRole("dialog", { name: "Review Changes" }) !== null;
}

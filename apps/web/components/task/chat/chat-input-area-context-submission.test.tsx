import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStore } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { FileBrowser } from "@/components/task/file-browser";
import { defaultState } from "@/lib/state/default-state";
import { useContextFilesStore, type ContextFile } from "@/lib/state/context-files-store";
import { MessageSendError } from "@/lib/chat/message-send-error";
import { useSubmitHandler } from "./chat-input-area";
import { useContextFiles, type ChatPanelState } from "./use-chat-panel-state";
import type { ChatSubmitResult } from "./chat-input-container";
import type { TaskSession } from "@/lib/types/http";
import { setWebSocketClient } from "@/lib/ws/connection";
import type { WebSocketClient } from "@/lib/ws/client";
import type { Window as HappyDOMWindow } from "happy-dom";

const transport = vi.hoisted(() => ({
  request: vi.fn(),
  getStatus: () => "connected",
  on: () => () => {},
  subscribeSession: () => () => {},
  subscribeUser: () => () => {},
}));

const SESSION = "context-submission-session" as TaskSession["id"];
const TASK = "context-submission-task" as TaskSession["task_id"];
const INCARNATION = "context-submission-incarnation";
const STORAGE_KEY = `kandev.contextFiles.${SESSION}`;
const SUBMITTED: ContextFile = { path: "submitted.ts", name: "submitted.ts" };
const NEXT: ContextFile = { path: "follow-up.ts", name: "follow-up.ts", isDirectory: false };
const PINNED: ContextFile = { path: "README.md", name: "README.md", pinned: true };
const DIRECT_ACTION = "message.add";
const STARTED_AT = "2026-10-07T00:00:00Z";
let submitter: ReturnType<typeof useSubmitHandler>;
const initialWidth = window.innerWidth;
const viewport = (window as unknown as HappyDOMWindow).happyDOM;

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

function SubmitHarness({ onSend }: { onSend?: () => ChatSubmitResult }) {
  const files = useContextFiles(SESSION);
  const queueReady = useAppStore(
    (state) =>
      state.queue.metaBySessionId[SESSION]?.sessionIncarnationId === INCARNATION &&
      state.queue.activeOperationBySessionId[SESSION] === undefined,
  );
  const panelState = {
    ...files,
    resolvedSessionId: SESSION,
    taskId: TASK,
    sessionModel: null,
    activeModel: null,
    activeDocument: null,
    planComments: [],
    previewFeedback: [],
    pendingPRFeedback: [],
    walkthroughComments: [],
    messageComments: [],
    pendingClarification: null,
    prompts: [],
    planModeEnabled: false,
    markCommentsSent: () => {},
    handleClearPRFeedback: () => {},
    handleClearWalkthroughComments: () => {},
  } as unknown as ChatPanelState;
  submitter = useSubmitHandler(panelState, onSend);
  return (
    <>
      <output data-testid="current-context">{JSON.stringify(files.contextFiles)}</output>
      <output data-testid="queue-ready">{String(queueReady)}</output>
    </>
  );
}

function mount(state: TaskSession["state"] = "WAITING_FOR_INPUT", onSend?: () => ChatSubmitResult) {
  const session: TaskSession = {
    id: SESSION,
    task_id: TASK,
    state,
    queue_incarnation_id: INCARNATION,
    started_at: STARTED_AT,
    updated_at: STARTED_AT,
  };
  vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
    const url =
      typeof input === "string" ? input : String(input instanceof URL ? input : input.url);
    if (!url.endsWith(`/api/v1/task-sessions/${SESSION}`)) {
      throw new Error(`Unexpected external HTTP request: ${url}`);
    }
    return new Response(JSON.stringify({ session }), {
      headers: { "Content-Type": "application/json", ETag: '"session-fixture"' },
    });
  });
  return render(
    <StateProvider
      initialState={{
        ...defaultState,
        connection: { ...defaultState.connection, status: "connected" },
        taskSessions: { ...defaultState.taskSessions, items: { [SESSION]: session } },
        sessionAgentctl: {
          ...defaultState.sessionAgentctl,
          itemsBySessionId: { [SESSION]: { status: "ready" } },
        },
        editors: { ...defaultState.editors, loaded: true, folderOpeningAvailable: false },
        userSettings: { ...defaultState.userSettings, loaded: true },
      }}
    >
      <ToastProvider>
        <TooltipProvider>
          <FileBrowser sessionId={SESSION} onOpenFile={() => {}} />
          <SubmitHarness onSend={onSend} />
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
}

async function addThroughFileBrowser() {
  fireEvent.click(await screen.findByRole("button", { name: "Search files" }));
  fireEvent.change(screen.getByRole("textbox"), { target: { value: NEXT.name } });
  await screen.findByText(NEXT.name);
  fireEvent.pointerDown(screen.getByTestId("file-tree-node-actions"));
  fireEvent.click(await screen.findByTestId("file-tree-touch-add-to-chat"));
  expect(liveFiles().map((f) => f.path)).toContain(NEXT.path);
}

function liveFiles() {
  return useContextFilesStore.getState().filesBySessionId[SESSION] ?? [];
}

function storedFiles(): ContextFile[] {
  return JSON.parse(window.sessionStorage.getItem(STORAGE_KEY) ?? "[]");
}

function startSubmission() {
  let completion!: Promise<boolean>;
  act(() => {
    completion = submitter.handleSubmit({ message: "Inspect the submitted selection" });
  });
  return completion;
}

async function waitForAdmission(action = DIRECT_ACTION) {
  await waitFor(() =>
    expect(transport.request.mock.calls.some(([name]) => name === action)).toBe(true),
  );
  expect(submitter.isSending).toBe(true);
}

function requestPayload(action = DIRECT_ACTION) {
  const calls = transport.request.mock.calls.filter(([name]) => name === action);
  expect(calls).toHaveLength(1);
  return calls[0][1] as { content: string; context_files: { path: string; name: string }[] };
}

beforeEach(() => {
  viewport.setWindowSize({ width: 390 });
  window.sessionStorage.clear();
  localStorage.clear();
  useContextFilesStore.setState({ filesBySessionId: {} });
  const store = useContextFilesStore.getState();
  setWebSocketClient(transport as unknown as WebSocketClient);
  store.addFile(SESSION, SUBMITTED);
  store.addFile(SESSION, PINNED);
  transport.request.mockReset().mockImplementation(async (action: string) => {
    if (action === "workspace.tree.get") {
      return {
        root: {
          path: "",
          name: "workspace",
          is_dir: true,
          children: [{ path: NEXT.path, name: NEXT.name, is_dir: false }],
        },
      };
    }
    if (action === "workspace.files.search") return { files: [NEXT.path] };
    if (action === "message.queue.get") {
      return {
        task_id: TASK,
        session_id: SESSION,
        session_incarnation_id: INCARNATION,
        entries: [],
        count: 0,
        max: 20,
      };
    }
    throw new Error(`Unexpected external request: ${action}`);
  });
});

afterEach(() => {
  cleanup();
  viewport.setWindowSize({ width: initialWidth });
  setWebSocketClient(null);
  useContextFilesStore.setState({ filesBySessionId: {} });
  window.sessionStorage.clear();
  localStorage.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function deferTransportAdmission(action = DIRECT_ACTION) {
  const admission = deferred<unknown>();
  const respond = transport.request.getMockImplementation()!;
  transport.request.mockImplementation((name: string, ...args: unknown[]) =>
    name === action ? admission.promise : respond(name, ...args),
  );
  return admission;
}

describe("submitted context ownership through real providers and callers", () => {
  // @covers AC-UI-FILE-TREE-CHAT-CONTEXT-001.6
  // @covers AC-UI-FILE-TREE-CHAT-CONTEXT-001.11
  it.each(["live", "storage", "hydration"])(
    "preserves FileBrowser's next selection after default direct admission in %s",
    async (boundary) => {
      const admission = deferTransportAdmission();
      mount();
      const completion = startSubmission();
      await waitForAdmission();
      await addThroughFileBrowser();
      const outgoing = requestPayload();
      expect(outgoing.context_files).toEqual([SUBMITTED, { path: PINNED.path, name: PINNED.name }]);
      expect(outgoing.content).toContain(SUBMITTED.path);
      expect(outgoing.content).not.toContain(NEXT.path);
      await act(async () => {
        admission.resolve(undefined);
        expect(await completion).toBe(true);
      });
      if (boundary === "hydration") {
        cleanup();
        useContextFilesStore.setState({ filesBySessionId: {} });
        useContextFilesStore.getState().hydrateSession(SESSION);
      }
      const remaining = boundary === "storage" ? storedFiles() : liveFiles();
      expect(remaining.map((file) => file.path)).toEqual([PINNED.path, NEXT.path]);
    },
  );

  // @covers AC-UI-FILE-TREE-CHAT-CONTEXT-001.5
  it("consumes unchanged submitted context while preserving pinned selections", async () => {
    const admission = deferTransportAdmission();
    mount();
    const completion = startSubmission();
    await waitForAdmission();
    await act(async () => {
      admission.resolve(undefined);
      expect(await completion).toBe(true);
    });
    expect(liveFiles()).toEqual([PINNED]);
    expect(storedFiles()).toEqual([PINNED]);
    expect(screen.getByTestId("current-context").textContent).toBe(JSON.stringify([PINNED]));
  });

  // @covers AC-UI-FILE-TREE-CHAT-CONTEXT-001.13
  it("preserves current old and new selections after default rejection", async () => {
    const admission = deferTransportAdmission();
    mount();
    const completion = startSubmission();
    await waitForAdmission();
    await addThroughFileBrowser();
    act(() => useContextFilesStore.getState().removeFile(SESSION, PINNED.path));
    await act(async () => {
      admission.reject(new MessageSendError("session-unavailable", "Admission rejected"));
      expect(await completion).toBe(false);
    });
    expect(liveFiles().map((file) => file.path)).toEqual([SUBMITTED.path, NEXT.path]);
    expect(storedFiles().map((file) => file.path)).toEqual([SUBMITTED.path, NEXT.path]);
  });
});

describe("pending submission selection edits and admission compatibility", () => {
  // @covers AC-UI-FILE-TREE-CHAT-CONTEXT-001.12
  it("preserves an equal-value same-path replacement after older acceptance", async () => {
    const admission = deferTransportAdmission();
    mount();
    const completion = startSubmission();
    await waitForAdmission();
    act(() => {
      useContextFilesStore.getState().removeFile(SESSION, SUBMITTED.path);
      useContextFilesStore.getState().addFile(SESSION, SUBMITTED);
    });
    await act(async () => {
      admission.resolve(undefined);
      expect(await completion).toBe(true);
    });
    expect(liveFiles()).toEqual([PINNED, SUBMITTED]);
    expect(storedFiles().map((file) => file.path)).toEqual([PINNED.path, SUBMITTED.path]);
  });

  // @covers AC-UI-FILE-TREE-CHAT-CONTEXT-001.14
  it("retains in-flight pin and unpin edits", async () => {
    const admission = deferTransportAdmission();
    mount();
    const completion = startSubmission();
    await waitForAdmission();
    act(() => {
      useContextFilesStore.getState().addFile(SESSION, { ...SUBMITTED, pinned: true });
      useContextFilesStore.getState().unpinFile(SESSION, PINNED.path);
    });
    await act(async () => {
      admission.resolve(undefined);
      expect(await completion).toBe(true);
    });
    expect(liveFiles()).toEqual([
      { ...SUBMITTED, pinned: true },
      { ...PINNED, pinned: false },
    ]);
    expect(storedFiles()).toEqual(liveFiles());
  });

  // @covers AC-UI-FILE-TREE-CHAT-CONTEXT-001.6
  // @covers AC-UI-FILE-TREE-CHAT-CONTEXT-001.11
  it("preserves later FileBrowser selections after real queued admission", async () => {
    const admission = deferTransportAdmission("message.queue.add");
    mount("RUNNING");
    await waitFor(() => expect(screen.getByTestId("queue-ready").textContent).toBe("true"));
    const completion = startSubmission();
    await waitForAdmission("message.queue.add");
    await addThroughFileBrowser();
    const outgoing = requestPayload("message.queue.add");
    expect(outgoing.context_files.map((file) => file.path)).toEqual([SUBMITTED.path, PINNED.path]);
    expect(outgoing.content).not.toContain(NEXT.path);
    await act(async () => {
      admission.resolve({ entry_id: "accepted-queue-entry" });
      expect(await completion).toBe(true);
    });
    expect(storedFiles().map((file) => file.path)).toEqual([PINNED.path, NEXT.path]);
    expect(liveFiles().map((file) => file.path)).toEqual([PINNED.path, NEXT.path]);
  });

  // @covers AC-UI-FILE-TREE-CHAT-CONTEXT-001.5
  // @covers AC-UI-FILE-TREE-CHAT-CONTEXT-001.13
  it.each([true, undefined, false, "throw"] as const)(
    "preserves callback admission compatibility for %s",
    async (outcome) => {
      const admission = deferred<void | boolean>();
      mount("WAITING_FOR_INPUT", () => admission.promise);
      const completion = startSubmission();
      expect(submitter.isSending).toBe(true);
      act(() => useContextFilesStore.getState().addFile(SESSION, NEXT));
      await act(async () => {
        if (outcome === "throw") admission.reject(new Error("Callback rejected"));
        else admission.resolve(outcome);
        expect(await completion).toBe(outcome !== false && outcome !== "throw");
      });
      const expected =
        outcome === false || outcome === "throw"
          ? [SUBMITTED.path, PINNED.path, NEXT.path]
          : [PINNED.path, NEXT.path];
      expect(liveFiles().map((file) => file.path)).toEqual(expected);
      expect(storedFiles().map((file) => file.path)).toEqual(expected);
      expect(transport.request.mock.calls.some(([name]) => name === DIRECT_ACTION)).toBe(false);
    },
  );
});

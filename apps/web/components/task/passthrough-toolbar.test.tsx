import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// --- Constants declared before vi.mock so factories can reference them ---
const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const SRC_FILE = "src/foo.ts";

// data-testid constants to avoid sonarjs/no-duplicate-string warnings
const TID_TOOLBAR = "passthrough-toolbar";
const TID_COMPOSER = "passthrough-composer";
const TID_TEXTAREA = "passthrough-composer-textarea";
const TID_PROCEED = "passthrough-proceed-next-step";
const TID_TOGGLE = "passthrough-toggle-composer";
const TID_TOGGLE_COMMENTS = "passthrough-toggle-comments";
const TID_COMMENTS_PANEL = "passthrough-comments-panel";
const TID_COMMENT_CARD = "passthrough-comment-card";
const TID_COMMENT_TEXTAREA = "passthrough-comment-textarea";
const TID_COMMENT_REMOVE = "passthrough-comment-remove";
const TID_COMMENT_FILE_REF = "passthrough-comment-file-ref";
const TID_PENDING_COUNT = "passthrough-pending-count";
const TID_PENDING_BANNER = "passthrough-pending-comments-banner";
const TID_PLAN_TOGGLE = "plan-mode-toggle-button";
const TID_ATTACHMENTS = "chat-attachments-button";
const TID_CONTEXT = "chat-context-button";

// --- Mutable state for per-test overrides ---
let mockSessionState: string | null = null;
let mockKeyboardShortcuts: Record<string, { key: string; modifiers?: Record<string, boolean> }> =
  {};
let mockPendingByFile: Record<string, import("@/lib/state/slices/comments").DiffComment[]> = {};
let mockPlanModeEnabled = false;
let mockImplementPlanHandler: ((fresh: boolean) => void) | undefined;
let mockNextStep: {
  proceedStepName: string | null;
  proceed: ReturnType<typeof vi.fn>;
  isMoving: boolean;
} = { proceedStepName: null, proceed: vi.fn(), isMoving: false };

const mockToast = vi.fn();
const mockMarkCommentsSent = vi.fn();
const mockUpdateComment = vi.fn();
const mockRemoveComment = vi.fn();
const mockOpenFile = vi.fn();
let mockWsRequestFn = vi.fn();
const chatInputMock = vi.hoisted(() => ({
  renderProps: vi.fn(),
  focusInput: vi.fn(),
  getTextareaElement: vi.fn(() => null as HTMLElement | null),
}));

// --- Module mocks (hoisted by Vitest) ---

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      taskSessions: {
        items: mockSessionState
          ? { [SESSION_ID]: { id: SESSION_ID, state: mockSessionState } }
          : {},
      },
      userSettings: { keyboardShortcuts: mockKeyboardShortcuts, chatSubmitKey: "enter" },
    }),
  useAppStoreApi: () => ({
    getState: () => ({
      kanban: { steps: [] },
      kanbanMulti: { snapshots: {} },
    }),
  }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/hooks/domains/kanban/use-plan-actions", () => ({
  usePlanActions: () => ({
    ...mockNextStep,
    implementPlanHandler: mockImplementPlanHandler,
  }),
}));

vi.mock("@/hooks/domains/comments/use-diff-comments", () => ({
  usePendingDiffCommentsByFile: () => mockPendingByFile,
}));

vi.mock("@/lib/state/slices/comments/comments-store", () => ({
  useCommentsStore: (
    selector: (s: {
      markCommentsSent: typeof mockMarkCommentsSent;
      updateComment: typeof mockUpdateComment;
      removeComment: typeof mockRemoveComment;
    }) => unknown,
  ) =>
    selector({
      markCommentsSent: mockMarkCommentsSent,
      updateComment: mockUpdateComment,
      removeComment: mockRemoveComment,
    }),
}));

vi.mock("@/hooks/use-file-editors", () => ({
  useFileEditors: () => ({ openFile: mockOpenFile }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mockWsRequestFn }),
}));

// Stub heavy sub-components that involve xterm / canvas / WebGL.
vi.mock("./passthrough-terminal", () => ({
  PassthroughTerminal: () => <div data-testid="passthrough-terminal-stub" />,
}));

vi.mock("@/components/github/pr-status-chip", () => ({
  PRStatusChip: () => null,
}));

vi.mock("@/components/azure-devops/azure-devops-task-pull-request-chip", () => ({
  AzureDevOpsTaskPullRequestChip: () => null,
}));

vi.mock("./chat/pr-archive-banners", () => ({
  PRMergedBanner: () => null,
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({
    asChild: _asChild,
    children,
  }: {
    asChild?: boolean;
    children: React.ReactNode;
  }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock("./chat/use-chat-panel-state", () => ({
  useChatPanelState: () => ({
    resolvedSessionId: SESSION_ID,
    taskId: TASK_ID,
    task: { id: TASK_ID, title: "Task title" },
    taskDescription: "Task description",
    planModeEnabled: mockPlanModeEnabled,
    planModeAvailable: true,
    mcpServers: ["kandev"],
    handlePlanModeChange: vi.fn(),
    isStarting: false,
    isPreparingEnvironment: false,
    contextItems: [{ id: "file:src/foo.ts", kind: "file", label: "foo.ts", filePath: SRC_FILE }],
    contextFiles: [{ path: SRC_FILE, name: "foo.ts" }],
    handleToggleContextFile: vi.fn(),
    handleAddContextFile: vi.fn(),
    addContextFile: vi.fn(),
    clearEphemeral: vi.fn(),
    planContextEnabled: false,
    chatSubmitKey: "enter",
    prompts: [{ id: "prompt-1", name: "Prompt", content: "Prompt content" }],
    planComments: [],
    pendingPRFeedback: [],
    walkthroughComments: [],
    messageComments: [],
    handleClearMessageComments: vi.fn(),
    pendingCommentsByFile: mockPendingByFile,
  }),
}));

vi.mock("./chat/chat-input-container", async () => {
  const React = await vi.importActual<typeof import("react")>("react");
  const ChatInputContainer = React.forwardRef<Record<string, unknown>, Record<string, unknown>>(
    function MockChatInputContainer(props, ref) {
      const [value, setValue] = React.useState("");
      chatInputMock.renderProps(props);
      React.useImperativeHandle(ref, () => ({
        focusInput: chatInputMock.focusInput,
        getTextareaElement: chatInputMock.getTextareaElement,
        getValue: () => value,
        getSelectionStart: () => value.length,
        insertText: vi.fn(),
        clear: vi.fn(),
        getAttachments: () => [],
      }));
      return (
        <div data-testid="mock-chat-input-container">
          {props.hasContextComments ? <div data-testid={TID_PENDING_BANNER} /> : null}
          <button type="button" data-testid={TID_PLAN_TOGGLE}>
            Plan
          </button>
          <button type="button" data-testid={TID_ATTACHMENTS}>
            Attach
          </button>
          <button type="button" data-testid={TID_CONTEXT}>
            Context
          </button>
          <textarea
            data-testid={TID_TEXTAREA}
            value={value}
            onChange={(event) => setValue(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Escape") {
                (props.onCancel as () => void)?.();
                return;
              }
              if (event.key === "Enter") {
                event.preventDefault();
                const result = (props.onSubmit as (message: string) => Promise<void> | void)?.(
                  value,
                );
                if (result && typeof result.then === "function") {
                  void result.catch(() => undefined);
                }
              }
            }}
          />
        </div>
      );
    },
  );
  return { ChatInputContainer };
});

// Import after mocks
import { PassthroughToolbar } from "./passthrough-toolbar";

// Helper to build a DiffComment with required fields
function makeDiffComment(id: string): import("@/lib/state/slices/comments").DiffComment {
  return {
    id,
    source: "diff",
    sessionId: SESSION_ID,
    filePath: SRC_FILE,
    startLine: 1,
    endLine: 1,
    side: "additions",
    codeContent: "const x = 1;",
    text: "Fix this",
    status: "pending",
    createdAt: new Date().toISOString(),
  };
}

function renderToolbar() {
  return render(<PassthroughToolbar sessionId={SESSION_ID} taskId={TASK_ID} />);
}

async function openComposer() {
  fireEvent.click(screen.getByTestId(TID_TOGGLE));
  await waitFor(() => expect(screen.getByTestId(TID_COMPOSER)).toBeTruthy());
}

function resetMocks() {
  mockSessionState = null;
  mockKeyboardShortcuts = {};
  mockPendingByFile = {};
  mockPlanModeEnabled = false;
  mockImplementPlanHandler = undefined;
  mockNextStep = { proceedStepName: null, proceed: vi.fn(), isMoving: false };
  mockWsRequestFn = vi.fn().mockResolvedValue(undefined);
  chatInputMock.renderProps.mockClear();
  vi.clearAllMocks();
}

function latestChatInputProps(): Record<string, unknown> {
  const calls = chatInputMock.renderProps.mock.calls;
  return (calls[calls.length - 1]?.[0] ?? {}) as Record<string, unknown>;
}

// ---------------------------------------------------------------------------
// Default / idle state
// ---------------------------------------------------------------------------

describe("PassthroughToolbar – default state", () => {
  afterEach(cleanup);

  it("renders the toolbar and hides the composer when session is idle", () => {
    mockSessionState = "IDLE";
    renderToolbar();

    expect(screen.getByTestId(TID_TOOLBAR)).toBeTruthy();
    expect(screen.queryByTestId(TID_COMPOSER)).toBeNull();
    expect(screen.queryByTestId(TID_PROCEED)).toBeNull();
  });

  it("disables the Comments toggle when there are no pending comments", () => {
    mockSessionState = "IDLE";
    renderToolbar();

    const toggle = screen.getByTestId(TID_TOGGLE_COMMENTS) as HTMLButtonElement;
    expect(toggle.disabled).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// Composer open / close
// ---------------------------------------------------------------------------

describe("PassthroughToolbar – composer toggle", () => {
  beforeEach(resetMocks);
  afterEach(cleanup);

  it("clicking Chat toggle opens and closes the composer", async () => {
    renderToolbar();
    const toggle = screen.getByTestId(TID_TOGGLE);
    expect(toggle.getAttribute("aria-pressed")).toBe("false");

    fireEvent.click(toggle);

    await waitFor(() => expect(screen.getByTestId(TID_COMPOSER)).toBeTruthy());
    expect(toggle.getAttribute("aria-pressed")).toBe("true");

    fireEvent.click(toggle);

    await waitFor(() => expect(screen.queryByTestId(TID_COMPOSER)).toBeNull());
    expect(toggle.getAttribute("aria-pressed")).toBe("false");
  });

  it("pressing Escape inside the composer closes it", async () => {
    renderToolbar();
    await openComposer();

    fireEvent.keyDown(screen.getByTestId(TID_TEXTAREA), { key: "Escape" });

    await waitFor(() => expect(screen.queryByTestId(TID_COMPOSER)).toBeNull());
  });

  it("wires passthrough composer controls and disables ACP-only controls", async () => {
    renderToolbar();
    await openComposer();

    expect(screen.getByTestId(TID_PLAN_TOGGLE)).toBeTruthy();
    expect(screen.getByTestId(TID_ATTACHMENTS)).toBeTruthy();
    expect(screen.getByTestId(TID_CONTEXT)).toBeTruthy();
    const props = latestChatInputProps();
    expect(props.hasAgentCommands).toBe(false);
    expect(props.hideAgentControls).toBe(true);
    expect(props.contextItems).toEqual(
      expect.arrayContaining([expect.objectContaining({ kind: "file", label: "foo.ts" })]),
    );
    expect(props.contextFiles).toEqual([{ path: SRC_FILE, name: "foo.ts" }]);
  });

  it("uses the passthrough-specific focus shortcut instead of the global slash shortcut", async () => {
    mockKeyboardShortcuts = {
      FOCUS_PASSTHROUGH_INPUT: { key: "y", modifiers: { ctrlOrCmd: true, shift: true } },
    };
    renderToolbar();

    fireEvent.keyDown(window, { key: "/", code: "Slash" });
    expect(screen.queryByTestId(TID_COMPOSER)).toBeNull();

    fireEvent.keyDown(window, { key: "y", code: "KeyY", ctrlKey: true, shiftKey: true });
    await waitFor(() => expect(screen.getByTestId(TID_COMPOSER)).toBeTruthy());
    expect(chatInputMock.focusInput).toHaveBeenCalled();
  });

  it("shows the passthrough chat shortcut in the Chat tooltip", () => {
    mockKeyboardShortcuts = {
      FOCUS_PASSTHROUGH_INPUT: { key: "y", modifiers: { ctrlOrCmd: true, shift: true } },
    };
    renderToolbar();

    expect(screen.getByText(/Ctrl\+Shift\+Y|Cmd\+Shift\+Y/)).toBeTruthy();
  });

  it("focus shortcut closes the composer when the composer textarea has focus", async () => {
    mockKeyboardShortcuts = {
      FOCUS_PASSTHROUGH_INPUT: { key: "y", modifiers: { ctrlOrCmd: true, shift: true } },
    };
    renderToolbar();

    fireEvent.keyDown(window, { key: "y", code: "KeyY", ctrlKey: true, shiftKey: true });
    await waitFor(() => expect(screen.getByTestId(TID_COMPOSER)).toBeTruthy());

    const textarea = screen.getByTestId(TID_TEXTAREA);
    textarea.focus();
    fireEvent.keyDown(window, { key: "y", code: "KeyY", ctrlKey: true, shiftKey: true });

    await waitFor(() => expect(screen.queryByTestId(TID_COMPOSER)).toBeNull());
  });

  it("does not steal focus from another editable field with the focus shortcut", () => {
    mockKeyboardShortcuts = {
      FOCUS_PASSTHROUGH_INPUT: { key: "y", modifiers: { ctrlOrCmd: true, shift: true } },
    };
    renderToolbar();
    const input = document.createElement("input");
    document.body.appendChild(input);
    try {
      input.focus();

      fireEvent.keyDown(window, { key: "y", code: "KeyY", ctrlKey: true, shiftKey: true });

      expect(screen.queryByTestId(TID_COMPOSER)).toBeNull();
      expect(chatInputMock.focusInput).not.toHaveBeenCalled();
    } finally {
      input.remove();
    }
  });

  it("focus shortcut works when xterm helper textarea owns focus", async () => {
    mockKeyboardShortcuts = {
      FOCUS_PASSTHROUGH_INPUT: { key: "y", modifiers: { ctrlOrCmd: true, shift: true } },
    };
    renderToolbar();
    const xterm = document.createElement("div");
    xterm.className = "xterm";
    const textarea = document.createElement("textarea");
    xterm.appendChild(textarea);
    document.body.appendChild(xterm);
    try {
      textarea.focus();

      fireEvent.keyDown(window, { key: "y", code: "KeyY", ctrlKey: true, shiftKey: true });

      await waitFor(() => expect(screen.getByTestId(TID_COMPOSER)).toBeTruthy());
      expect(chatInputMock.focusInput).toHaveBeenCalled();
    } finally {
      xterm.remove();
    }
  });
});

// ---------------------------------------------------------------------------
// Implement plan handler
// ---------------------------------------------------------------------------

describe("PassthroughToolbar – implement plan handler", () => {
  beforeEach(resetMocks);
  afterEach(cleanup);

  it("forwards the implement handler to the composer while plan mode is active", async () => {
    const implementHandler = vi.fn();
    mockPlanModeEnabled = true;
    mockImplementPlanHandler = implementHandler;

    renderToolbar();
    await openComposer();

    expect(latestChatInputProps().planModeEnabled).toBe(true);
    expect(latestChatInputProps().onImplementPlan).toBe(implementHandler);
  });

  it("does not forward an implement handler when plan mode is disabled", async () => {
    mockPlanModeEnabled = false;
    mockImplementPlanHandler = vi.fn();

    renderToolbar();
    await openComposer();

    expect(latestChatInputProps().planModeEnabled).toBe(false);
    expect(latestChatInputProps().onImplementPlan).toBeUndefined();
  });

  it("does not forward an implement handler while the passthrough session is busy", async () => {
    mockPlanModeEnabled = true;
    mockImplementPlanHandler = vi.fn();
    mockSessionState = "RUNNING";

    renderToolbar();
    await openComposer();

    expect(latestChatInputProps().planModeEnabled).toBe(true);
    expect(latestChatInputProps().onImplementPlan).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// Send message
// ---------------------------------------------------------------------------

describe("PassthroughToolbar – send message", () => {
  beforeEach(resetMocks);
  afterEach(cleanup);

  it("send with no pending comments calls message.add with exact text and closes composer", async () => {
    renderToolbar();
    await openComposer();

    fireEvent.change(screen.getByTestId(TID_TEXTAREA), { target: { value: "hello" } });
    fireEvent.keyDown(screen.getByTestId(TID_TEXTAREA), { key: "Enter" });

    await waitFor(() =>
      expect(mockWsRequestFn).toHaveBeenCalledWith(
        "message.add",
        expect.objectContaining({
          task_id: TASK_ID,
          session_id: SESSION_ID,
          content: expect.stringContaining("hello"),
          context_files: [{ path: SRC_FILE, name: "foo.ts" }],
        }),
        10_000,
      ),
    );
    await waitFor(() => expect(screen.queryByTestId(TID_COMPOSER)).toBeNull());
    expect(mockMarkCommentsSent).not.toHaveBeenCalled();
  });

  it("send with pending comments prepends review markdown and calls markCommentsSent", async () => {
    mockPendingByFile = { [SRC_FILE]: [makeDiffComment("c1")] };
    renderToolbar();
    await openComposer();

    fireEvent.change(screen.getByTestId(TID_TEXTAREA), { target: { value: "ship it" } });
    fireEvent.keyDown(screen.getByTestId(TID_TEXTAREA), { key: "Enter" });

    await waitFor(() => expect(mockWsRequestFn).toHaveBeenCalledTimes(1));

    const content = mockWsRequestFn.mock.calls[0][1].content as string;
    expect(content).toMatch(/^### Review Comments\n/);
    expect(content).toContain("ship it");
    expect(content).toContain("CONTEXT FILES");

    await waitFor(() => expect(mockMarkCommentsSent).toHaveBeenCalledWith(["c1"]));
  });

  it("on failure keeps the composer open, shows an error toast, and does not call markCommentsSent", async () => {
    let rejectSend!: (err: Error) => void;
    mockWsRequestFn = vi.fn().mockReturnValue(new Promise<void>((_, rej) => (rejectSend = rej)));

    renderToolbar();
    await openComposer();

    fireEvent.change(screen.getByTestId(TID_TEXTAREA), { target: { value: "important" } });
    fireEvent.keyDown(screen.getByTestId(TID_TEXTAREA), { key: "Enter" });

    rejectSend(new Error("network down"));

    await waitFor(() =>
      expect(mockToast).toHaveBeenCalledWith(expect.objectContaining({ variant: "error" })),
    );
    expect(screen.getByTestId(TID_COMPOSER)).toBeTruthy();
    expect(mockMarkCommentsSent).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// Pending-comment indicators
// ---------------------------------------------------------------------------

describe("PassthroughToolbar – pending comment indicators", () => {
  beforeEach(resetMocks);
  afterEach(cleanup);

  it("shows a numeric chip when the composer is collapsed and comments are pending", () => {
    mockPendingByFile = {
      [SRC_FILE]: [makeDiffComment("c1"), makeDiffComment("c2"), makeDiffComment("c3")],
    };
    renderToolbar();

    expect(screen.queryByTestId(TID_COMPOSER)).toBeNull();
    expect(screen.getByTestId(TID_PENDING_COUNT).textContent).toBe("3");
  });

  it("renders the pending-comments banner inside the open composer when comments are pending", async () => {
    mockPendingByFile = { [SRC_FILE]: [makeDiffComment("c1"), makeDiffComment("c2")] };
    renderToolbar();
    await openComposer();

    expect(latestChatInputProps().pendingCommentsByFile).toEqual(mockPendingByFile);
  });
});

// ---------------------------------------------------------------------------
// Comments panel
// ---------------------------------------------------------------------------

describe("PassthroughToolbar – Comments panel", () => {
  beforeEach(resetMocks);
  afterEach(cleanup);

  it("clicking Comments opens a panel with one card per pending comment", async () => {
    mockPendingByFile = {
      [SRC_FILE]: [makeDiffComment("c1"), makeDiffComment("c2")],
    };
    renderToolbar();

    fireEvent.click(screen.getByTestId(TID_TOGGLE_COMMENTS));

    await waitFor(() => expect(screen.getByTestId(TID_COMMENTS_PANEL)).toBeTruthy());
    expect(screen.getAllByTestId(TID_COMMENT_CARD)).toHaveLength(2);
  });

  it("editing a comment textarea calls updateComment with the new text", async () => {
    mockPendingByFile = { [SRC_FILE]: [makeDiffComment("c1")] };
    renderToolbar();

    fireEvent.click(screen.getByTestId(TID_TOGGLE_COMMENTS));
    await waitFor(() => expect(screen.getByTestId(TID_COMMENTS_PANEL)).toBeTruthy());

    fireEvent.change(screen.getByTestId(TID_COMMENT_TEXTAREA), {
      target: { value: "Updated text" },
    });

    expect(mockUpdateComment).toHaveBeenCalledWith("c1", { text: "Updated text" });
  });

  it("clicking the file reference opens the file in an editor", async () => {
    mockPendingByFile = { [SRC_FILE]: [makeDiffComment("c1")] };
    renderToolbar();

    fireEvent.click(screen.getByTestId(TID_TOGGLE_COMMENTS));
    await waitFor(() => expect(screen.getByTestId(TID_COMMENTS_PANEL)).toBeTruthy());

    fireEvent.click(screen.getByTestId(TID_COMMENT_FILE_REF));

    expect(mockOpenFile).toHaveBeenCalledWith(SRC_FILE);
  });

  it("clicking the remove button on a comment calls removeComment", async () => {
    mockPendingByFile = { [SRC_FILE]: [makeDiffComment("c1")] };
    renderToolbar();

    fireEvent.click(screen.getByTestId(TID_TOGGLE_COMMENTS));
    await waitFor(() => expect(screen.getByTestId(TID_COMMENTS_PANEL)).toBeTruthy());

    fireEvent.click(screen.getByTestId(TID_COMMENT_REMOVE));

    expect(mockRemoveComment).toHaveBeenCalledWith("c1");
  });
});

// ---------------------------------------------------------------------------
// Proceed-next-step button
// ---------------------------------------------------------------------------

describe("PassthroughToolbar – proceed button", () => {
  beforeEach(resetMocks);
  afterEach(cleanup);

  it("is absent when nextStepName is null", () => {
    mockNextStep = { proceedStepName: null, proceed: vi.fn(), isMoving: false };
    mockSessionState = "IDLE";
    renderToolbar();
    expect(screen.queryByTestId(TID_PROCEED)).toBeNull();
  });

  it("is absent when nextStepName is set but the agent is RUNNING", () => {
    mockNextStep = { proceedStepName: "Review", proceed: vi.fn(), isMoving: false };
    mockSessionState = "RUNNING";
    renderToolbar();
    expect(screen.queryByTestId(TID_PROCEED)).toBeNull();
  });

  it("is present with correct label and calls proceed on click when agent is idle", async () => {
    const proceedFn = vi.fn();
    mockNextStep = { proceedStepName: "Review", proceed: proceedFn, isMoving: false };
    mockSessionState = "IDLE";
    renderToolbar();

    const btn = screen.getByTestId(TID_PROCEED);
    expect(btn.textContent).toMatch(/Review/);

    fireEvent.click(btn);
    await waitFor(() => expect(proceedFn).toHaveBeenCalledTimes(1));
  });
});

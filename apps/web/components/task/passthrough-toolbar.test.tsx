import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// --- Constants declared before vi.mock so factories can reference them ---
const TASK_ID = "task-1";
const SESSION_ID = "session-1";

// data-testid constants to avoid sonarjs/no-duplicate-string warnings
const TID_TOOLBAR = "passthrough-toolbar";
const TID_COMPOSER = "passthrough-composer";
const TID_TEXTAREA = "passthrough-composer-textarea";
const TID_STOP = "passthrough-stop";
const TID_PROCEED = "passthrough-proceed-next-step";
const TID_TOGGLE = "passthrough-toggle-composer";
const TID_PENDING_COUNT = "passthrough-pending-count";
const TID_PENDING_BANNER = "passthrough-pending-comments-banner";

// --- Mutable state for per-test overrides ---
let mockSessionState: string | null = null;
let mockPendingByFile: Record<string, import("@/lib/state/slices/comments").DiffComment[]> = {};
let mockNextStep: {
  proceedStepName: string | null;
  proceed: ReturnType<typeof vi.fn>;
  isMoving: boolean;
} = { proceedStepName: null, proceed: vi.fn(), isMoving: false };

const mockToast = vi.fn();
const mockMarkCommentsSent = vi.fn();
let mockWsRequestFn = vi.fn();

// --- Module mocks (hoisted by Vitest) ---

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      taskSessions: {
        items: mockSessionState
          ? { [SESSION_ID]: { id: SESSION_ID, state: mockSessionState } }
          : {},
      },
    }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/hooks/domains/kanban/use-plan-actions", () => ({
  useNextWorkflowStep: () => mockNextStep,
}));

vi.mock("@/hooks/domains/comments/use-diff-comments", () => ({
  usePendingDiffCommentsByFile: () => mockPendingByFile,
}));

vi.mock("@/lib/state/slices/comments/comments-store", () => ({
  useCommentsStore: (selector: (s: { markCommentsSent: typeof mockMarkCommentsSent }) => unknown) =>
    selector({ markCommentsSent: mockMarkCommentsSent }),
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

vi.mock("./chat/chat-input-area", () => ({
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

// Import after mocks
import { PassthroughToolbar } from "./passthrough-toolbar";

// Helper to build a DiffComment with required fields
function makeDiffComment(id: string): import("@/lib/state/slices/comments").DiffComment {
  return {
    id,
    source: "diff",
    sessionId: SESSION_ID,
    filePath: "src/foo.ts",
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
  mockPendingByFile = {};
  mockNextStep = { proceedStepName: null, proceed: vi.fn(), isMoving: false };
  mockWsRequestFn = vi.fn().mockResolvedValue(undefined);
  vi.clearAllMocks();
}

// ---------------------------------------------------------------------------
// Default / idle state
// ---------------------------------------------------------------------------

describe("PassthroughToolbar – default state", () => {
  afterEach(cleanup);

  it("renders the toolbar, hides the composer, and disables Stop when session is idle", () => {
    mockSessionState = "IDLE";
    renderToolbar();

    expect(screen.getByTestId(TID_TOOLBAR)).toBeTruthy();
    expect(screen.queryByTestId(TID_COMPOSER)).toBeNull();
    expect((screen.getByTestId(TID_STOP) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.queryByTestId(TID_PROCEED)).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Composer open / close
// ---------------------------------------------------------------------------

describe("PassthroughToolbar – composer toggle", () => {
  beforeEach(resetMocks);
  afterEach(cleanup);

  it("clicking Chat toggle opens the composer and sets aria-pressed=true", async () => {
    renderToolbar();
    const toggle = screen.getByTestId(TID_TOGGLE);
    expect(toggle.getAttribute("aria-pressed")).toBe("false");

    fireEvent.click(toggle);

    await waitFor(() => expect(screen.getByTestId(TID_COMPOSER)).toBeTruthy());
    expect(toggle.getAttribute("aria-pressed")).toBe("true");
  });

  it("pressing Escape inside the composer closes it", async () => {
    renderToolbar();
    await openComposer();

    fireEvent.keyDown(screen.getByTestId(TID_TEXTAREA), { key: "Escape" });

    await waitFor(() => expect(screen.queryByTestId(TID_COMPOSER)).toBeNull());
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
        { task_id: TASK_ID, session_id: SESSION_ID, content: "hello" },
        10_000,
      ),
    );
    await waitFor(() => expect(screen.queryByTestId(TID_COMPOSER)).toBeNull());
    expect(mockMarkCommentsSent).not.toHaveBeenCalled();
  });

  it("send with pending comments prepends review markdown and calls markCommentsSent", async () => {
    mockPendingByFile = { "src/foo.ts": [makeDiffComment("c1")] };
    renderToolbar();
    await openComposer();

    fireEvent.change(screen.getByTestId(TID_TEXTAREA), { target: { value: "ship it" } });
    fireEvent.keyDown(screen.getByTestId(TID_TEXTAREA), { key: "Enter" });

    await waitFor(() => expect(mockWsRequestFn).toHaveBeenCalledTimes(1));

    const content = mockWsRequestFn.mock.calls[0][1].content as string;
    expect(content).toMatch(/^### Review Comments\n/);
    expect(content).toMatch(/ship it$/);

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
      "src/foo.ts": [makeDiffComment("c1"), makeDiffComment("c2"), makeDiffComment("c3")],
    };
    renderToolbar();

    expect(screen.queryByTestId(TID_COMPOSER)).toBeNull();
    expect(screen.getByTestId(TID_PENDING_COUNT).textContent).toBe("3");
  });

  it("renders the pending-comments banner inside the open composer when comments are pending", async () => {
    mockPendingByFile = { "src/foo.ts": [makeDiffComment("c1"), makeDiffComment("c2")] };
    renderToolbar();
    await openComposer();

    const banner = screen.getByTestId(TID_PENDING_BANNER);
    expect(banner.textContent).toMatch(/2/);
  });
});

// ---------------------------------------------------------------------------
// Stop button
// ---------------------------------------------------------------------------

describe("PassthroughToolbar – Stop button", () => {
  beforeEach(resetMocks);
  afterEach(cleanup);

  it("Stop is enabled when RUNNING, calls agent.cancel, and disables during the in-flight call", async () => {
    mockSessionState = "RUNNING";

    let resolveCancel!: () => void;
    mockWsRequestFn = vi.fn().mockReturnValue(new Promise<void>((res) => (resolveCancel = res)));
    renderToolbar();

    const stop = () => screen.getByTestId(TID_STOP) as HTMLButtonElement;
    expect(stop().disabled).toBe(false);

    fireEvent.click(stop());
    await waitFor(() => expect(stop().disabled).toBe(true));

    expect(mockWsRequestFn).toHaveBeenCalledWith(
      "agent.cancel",
      { session_id: SESSION_ID },
      10_000,
    );

    resolveCancel();
    await waitFor(() => expect(stop().disabled).toBe(false));
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

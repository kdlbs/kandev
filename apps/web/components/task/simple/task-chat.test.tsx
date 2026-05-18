import { describe, it, expect, afterEach, vi } from "vitest";
import { render, screen, cleanup, fireEvent, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import type {
  TaskComment,
  TaskDecision,
  TaskSession,
  TimelineEvent,
} from "@/app/office/tasks/[id]/types";

// Stub out everything that would otherwise drag the WS layer / portals into
// the test runtime. We assert merge/order, "Show older sessions", and that
// session entries appear at the right position in the unified list.

vi.mock("@/app/office/tasks/[id]/advanced-panels/chat-panel", () => ({
  AdvancedChatPanel: ({ sessionId }: { sessionId: string | null }) => (
    <div data-testid={`embed-${sessionId ?? "none"}`} />
  ),
}));

vi.mock("@/components/enhance-prompt-button", () => ({
  EnhancePromptButton: () => null,
}));

vi.mock("@/hooks/use-is-utility-configured", () => ({
  useIsUtilityConfigured: () => false,
}));

vi.mock("@/hooks/use-utility-agent-generator", () => ({
  useUtilityAgentGenerator: () => ({ enhancePrompt: () => {}, isEnhancingPrompt: false }),
}));

vi.mock("./markdown-comment", () => ({
  MarkdownComment: ({ content }: { content: string }) => <p>{content}</p>,
}));

import { createContext, useContext } from "react";
const CollapsibleOpenCtx = createContext<{
  open: boolean;
  onOpenChange?: (next: boolean) => void;
}>({ open: true });

vi.mock("@kandev/ui/collapsible", () => ({
  Collapsible: ({
    open,
    onOpenChange,
    children,
  }: {
    open?: boolean;
    onOpenChange?: (v: boolean) => void;
    children: ReactNode;
  }) => (
    <CollapsibleOpenCtx.Provider value={{ open: !!open, onOpenChange }}>
      <div data-open={open}>{children}</div>
    </CollapsibleOpenCtx.Provider>
  ),
  CollapsibleTrigger: ({ children, className }: { children: ReactNode; className?: string }) => {
    const { open, onOpenChange } = useContext(CollapsibleOpenCtx);
    return (
      <button type="button" onClick={() => onOpenChange?.(!open)} className={className}>
        {children}
      </button>
    );
  },
  CollapsibleContent: ({ children }: { children: ReactNode }) => {
    const { open } = useContext(CollapsibleOpenCtx);
    if (!open) return null;
    return <>{children}</>;
  },
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("@kandev/ui/button", () => ({
  Button: ({ children, ...rest }: { children: ReactNode } & Record<string, unknown>) => (
    <button {...rest}>{children}</button>
  ),
}));

import { TaskChat } from "./task-chat";
import { ActiveSessionRefProvider } from "./components/active-session-ref-context";

afterEach(() => cleanup());

const T_10 = "2026-05-01T10:00:00Z";
const T_11 = "2026-05-01T11:00:00Z";
const T_AGENT_A1 = "2026-05-01T10:30:00Z";

function makeComment(id: string, createdAt: string, content = `c-${id}`): TaskComment {
  return {
    id,
    taskId: "task-1",
    authorType: "user",
    authorId: "u-1",
    authorName: "You",
    content,
    createdAt,
  };
}

function makeSession(id: string, startedAt: string, state: TaskSession["state"]): TaskSession {
  return {
    id,
    agentName: "Alice",
    agentRole: "agent",
    state,
    isPrimary: true,
    startedAt,
    updatedAt: startedAt,
  };
}

function wrap(node: ReactNode) {
  return (
    <StateProvider>
      <ActiveSessionRefProvider>{node}</ActiveSessionRefProvider>
    </StateProvider>
  );
}

describe("TaskChat unified timeline", () => {
  // Session entries are intentionally not rendered in the Chat tab —
  // each agent message below already shows the same agent name +
  // "worked for Xs" footer, so the collapsed session header would be
  // pure duplicate noise. The per-agent sibling tabs render the full
  // transcript when the user wants it. The tests below assert the
  // remaining timeline contract (comments + decisions + timeline
  // events) and the "Show older sessions" toggle, which still gates
  // whether the older session groups participate in chat-derived
  // counts even though their entries don't render here.
  it("interleaves comments and timeline events by timestamp", () => {
    const comments: TaskComment[] = [
      makeComment("c-early", T_10, "early"),
      makeComment("c-late", "2026-05-01T12:00:00Z", "late"),
    ];
    const timeline: TimelineEvent[] = [
      { type: "status_change", from: "todo", to: "in_progress", at: T_AGENT_A1 },
    ];

    render(
      wrap(<TaskChat taskId="task-1" comments={comments} timeline={timeline} sessions={[]} />),
    );

    const root = screen.getByTestId("task-chat-entries");
    const html = root.innerHTML;
    const earlyIdx = html.indexOf("early");
    const statusIdx = html.indexOf("Status changed");
    const lateIdx = html.indexOf("late");

    expect(earlyIdx).toBeGreaterThanOrEqual(0);
    expect(statusIdx).toBeGreaterThan(earlyIdx);
    expect(lateIdx).toBeGreaterThan(statusIdx);
  });

  it("renders 'Show older sessions' when more than 50 sessions exist and hides the toggle on click", () => {
    const sessions: TaskSession[] = [];
    // 53 sessions: 3 older, then 50 visible.
    for (let i = 0; i < 53; i++) {
      const stamp = new Date(2026, 4, 1, 0, i, 0).toISOString();
      sessions.push(makeSession(`s-${i.toString().padStart(2, "0")}`, stamp, "COMPLETED"));
    }

    render(wrap(<TaskChat taskId="task-1" comments={[]} sessions={sessions} />));

    const toggle = screen.getByTestId("show-older-sessions");
    expect(within(toggle).getByText(/Show 3 older sessions/)).toBeTruthy();

    fireEvent.click(toggle);

    // After expansion the toggle disappears; the older session group
    // is now in scope even though no session-timeline-entry markup is
    // rendered in the Chat tab (see comment above).
    expect(screen.queryByTestId("show-older-sessions")).toBeNull();
  });

  it("does not render 'Show older sessions' when there are <= 50 sessions", () => {
    const sessions: TaskSession[] = [];
    for (let i = 0; i < 5; i++) {
      const stamp = new Date(2026, 4, 1, 0, i, 0).toISOString();
      sessions.push(makeSession(`s-${i}`, stamp, "COMPLETED"));
    }
    render(wrap(<TaskChat taskId="task-1" comments={[]} sessions={sessions} />));
    expect(screen.queryByTestId("show-older-sessions")).toBeNull();
  });

  it("renders the input area when not read-only", () => {
    render(wrap(<TaskChat taskId="task-1" comments={[]} sessions={[]} />));
    expect(screen.getByPlaceholderText("Add a comment...")).toBeTruthy();
  });

  it("hides the input area when read-only", () => {
    render(wrap(<TaskChat taskId="task-1" comments={[]} sessions={[]} readOnly />));
    expect(screen.queryByPlaceholderText("Add a comment...")).toBeNull();
  });
});

describe("TaskChat office vs kanban grouping", () => {
  function officeSession(
    id: string,
    agentProfileId: string,
    state: TaskSession["state"],
    startedAt: string,
    agentName = "Agent",
  ): TaskSession {
    return {
      id,
      agentProfileId,
      agentName,
      agentRole: "agent",
      state,
      isPrimary: false,
      startedAt,
      updatedAt: startedAt,
    };
  }

  // Office (per-agent) and kanban sessions are NOT rendered inline in
  // the Chat tab — the agent messages already carry the same info and
  // the per-agent sibling tabs render the full transcript. The tests
  // below pin that contract: presence of office or kanban sessions
  // does not produce a `session-timeline-entry-*` element in the Chat
  // root.
  it("does not render office session entries inline in chat", () => {
    const sessions: TaskSession[] = [
      officeSession("s-ceo", "agent-ceo", "IDLE", T_10, "CEO"),
      officeSession("s-rev", "agent-rev", "RUNNING", T_11, "QA"),
    ];
    render(
      wrap(
        <TaskChat taskId="task-1" comments={[]} sessions={sessions} reviewers={["agent-rev"]} />,
      ),
    );
    expect(screen.queryByTestId("session-timeline-entry-s-ceo")).toBeNull();
    expect(screen.queryByTestId("session-timeline-entry-s-rev")).toBeNull();
  });

  it("does not render kanban session entries inline in chat", () => {
    const sessions: TaskSession[] = [
      makeSession("s-1", T_10, "COMPLETED"),
      makeSession("s-2", T_11, "RUNNING"),
    ];
    render(wrap(<TaskChat taskId="task-1" comments={[]} sessions={sessions} />));
    expect(screen.queryByTestId("session-timeline-entry-s-1")).toBeNull();
    expect(screen.queryByTestId("session-timeline-entry-s-2")).toBeNull();
  });
});

describe("TaskChat decisions in timeline", () => {
  it("renders decision entries interleaved with comments", () => {
    const comments: TaskComment[] = [
      makeComment("c-early", T_10, "early"),
      makeComment("c-late", "2026-05-01T13:00:00Z", "late"),
    ];
    const decisions: TaskDecision[] = [
      {
        id: "d1",
        taskId: "task-1",
        deciderType: "agent",
        deciderId: "a1",
        deciderName: "CEO",
        role: "approver",
        decision: "approved",
        comment: "",
        createdAt: T_11,
      },
      {
        id: "d2",
        taskId: "task-1",
        deciderType: "agent",
        deciderId: "a2",
        deciderName: "Eng Lead",
        role: "approver",
        decision: "changes_requested",
        comment: "please update the docs",
        createdAt: "2026-05-01T12:00:00Z",
      },
    ];

    render(
      wrap(<TaskChat taskId="task-1" comments={comments} sessions={[]} decisions={decisions} />),
    );

    const root = screen.getByTestId("task-chat-entries");
    const html = root.innerHTML;
    const earlyIdx = html.indexOf("early");
    const ceoIdx = html.indexOf("CEO approved");
    const engIdx = html.indexOf("Eng Lead requested changes");
    const lateIdx = html.indexOf("late");

    expect(earlyIdx).toBeGreaterThanOrEqual(0);
    expect(ceoIdx).toBeGreaterThan(earlyIdx);
    expect(engIdx).toBeGreaterThan(ceoIdx);
    expect(lateIdx).toBeGreaterThan(engIdx);
    expect(html).toContain("please update the docs");
  });
});

function agentSessionComment(id: string, authorId: string, createdAt: string): TaskComment {
  return {
    id,
    taskId: "task-1",
    authorType: "agent",
    authorId,
    authorName: "",
    content: `body-${id}`,
    source: "session",
    createdAt,
  };
}
function userTurnComment(id: string, createdAt: string): TaskComment {
  return {
    id,
    taskId: "task-1",
    authorType: "user",
    authorId: "u",
    authorName: "You",
    content: `q-${id}`,
    createdAt,
  };
}
function officeTurnSession(
  id: string,
  agentId: string,
  state: TaskSession["state"],
  startedAt: string,
): TaskSession {
  return {
    id,
    agentProfileId: agentId,
    agentName: "Agent",
    agentRole: "agent",
    state,
    isPrimary: false,
    startedAt,
    updatedAt: startedAt,
  };
}

describe("buildCommentTurnContext", () => {
  // Pins per-comment slicing: every comment (user or agent) maps to a
  // turn window scoped to (previous comment OR session.startedAt, this
  // comment]. The chat renders one collapsible AgentTurnPanel per
  // comment so each reply carries the work that produced it.
  it("scopes each comment to a turn window in the session", async () => {
    const { buildCommentTurnContext } = await import("./turn-context");
    const sessions = [officeTurnSession("s-1", "agent-a", "IDLE", T_10)];
    const comments = [
      agentSessionComment("a1", "agent-a", T_AGENT_A1),
      userTurnComment("u1", "2026-05-01T10:45:00Z"),
      agentSessionComment("a2", "agent-a", "2026-05-01T11:00:00Z"),
    ];
    const ctx = buildCommentTurnContext(comments, sessions);
    expect(ctx.get("a1")).toEqual({
      sessionId: "s-1",
      fromExclusive: T_10,
      toInclusive: T_AGENT_A1,
    });
    expect(ctx.get("u1")).toEqual({
      sessionId: "s-1",
      fromExclusive: T_AGENT_A1,
      toInclusive: "2026-05-01T10:45:00Z",
    });
    expect(ctx.get("a2")).toEqual({
      sessionId: "s-1",
      fromExclusive: "2026-05-01T10:45:00Z",
      toInclusive: "2026-05-01T11:00:00Z",
    });
  });
});

describe("TaskChat user comment run badge", () => {
  function userQueuedComment(
    id: string,
    createdAt: string,
    runStatus: TaskComment["runStatus"] = "queued",
  ): TaskComment {
    return {
      id,
      taskId: "task-1",
      authorType: "user",
      authorId: "u",
      authorName: "You",
      content: `q-${id}`,
      createdAt,
      runId: `run-${id}`,
      runStatus,
    };
  }

  it("renders the run badge for a user comment with no later agent reply", () => {
    render(
      wrap(<TaskChat taskId="task-1" comments={[userQueuedComment("c1", T_10)]} sessions={[]} />),
    );
    const badge = screen.getByTestId("user-comment-run-badge");
    expect(badge.getAttribute("data-status")).toBe("queued");
  });

  it("hides the run badge once an agent reply with a later timestamp exists", () => {
    const comments: TaskComment[] = [
      userQueuedComment("c1", T_10),
      {
        id: "a1",
        taskId: "task-1",
        authorType: "agent",
        authorId: "agent-x",
        authorName: "",
        content: "reply",
        source: "session",
        createdAt: T_11,
      },
    ];
    render(wrap(<TaskChat taskId="task-1" comments={comments} sessions={[]} />));
    expect(screen.queryByTestId("user-comment-run-badge")).toBeNull();
  });
});

describe("TaskChat run error entries", () => {
  function failedSession(
    id: string,
    agentProfileId: string,
    completedAt: string,
    errorMessage: string,
  ): TaskSession {
    return {
      id,
      agentProfileId,
      agentName: "Agent",
      agentRole: "agent",
      state: "FAILED",
      isPrimary: false,
      startedAt: T_10,
      completedAt,
      updatedAt: completedAt,
      errorMessage,
    };
  }

  // Pins the new chat error entry: a FAILED office session is rendered
  // as a structured top-level entry with a generic header and the raw
  // payload behind a Show details collapsible — replacing the legacy
  // red JSON-RPC blob banner.
  it("renders a structured error entry for FAILED office sessions", () => {
    const sessions: TaskSession[] = [
      failedSession(
        "s-broken",
        "agent-broken",
        "2026-05-01T11:30:00Z",
        '{"code":-32603,"message":"Internal error"}',
      ),
    ];
    render(wrap(<TaskChat taskId="task-1" comments={[]} sessions={sessions} />));
    expect(screen.getByText(/agent stopped with an error/i)).toBeTruthy();
    expect(screen.getByTestId("run-error-resume-button")).toBeTruthy();
    expect(screen.getByTestId("run-error-fresh-button")).toBeTruthy();
    // Show details starts collapsed; click reveals the raw payload.
    fireEvent.click(screen.getByText(/show details/i));
    expect(screen.getByTestId("run-error-raw-payload").textContent).toContain("Internal error");
  });

  it("does not render an error entry for non-FAILED sessions", () => {
    const sessions: TaskSession[] = [
      {
        id: "s-ok",
        agentProfileId: "agent-ok",
        agentName: "Agent",
        agentRole: "agent",
        state: "IDLE",
        isPrimary: false,
        startedAt: T_10,
        updatedAt: T_10,
      },
    ];
    render(wrap(<TaskChat taskId="task-1" comments={[]} sessions={sessions} />));
    expect(screen.queryByText(/agent stopped with an error/i)).toBeNull();
    expect(screen.queryByTestId("run-error-raw-payload")).toBeNull();
  });
});

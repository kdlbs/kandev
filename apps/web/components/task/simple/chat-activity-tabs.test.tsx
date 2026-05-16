import { describe, it, expect, afterEach, vi } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import type { Task, TaskSession } from "@/app/office/tasks/[id]/types";

// Stub heavy descendants so the test stays focused on the tabs and the
// live-dot logic. AdvancedChatPanel renders WS-driven hook trees we don't
// want here; TaskChat / TaskActivity have their own coverage.
vi.mock("@/app/office/tasks/[id]/advanced-panels/chat-panel", () => ({
  AdvancedChatPanel: ({ sessionId }: { sessionId: string | null }) => (
    <div data-testid={`embed-${sessionId ?? "none"}`} />
  ),
}));

vi.mock("./task-chat", () => ({
  TaskChat: () => <div data-testid="task-chat-stub" />,
}));

vi.mock("./task-activity", () => ({
  TaskActivity: () => <div data-testid="task-activity-stub" />,
}));

vi.mock("./components/approval-action-bar", () => ({
  ApprovalActionBar: () => null,
}));

import { ChatActivityTabs } from "./chat-activity-tabs";

afterEach(() => cleanup());

const T_10 = "2026-05-01T10:00:00Z";
const T_11 = "2026-05-01T11:00:00Z";
const T_12 = "2026-05-01T12:00:00Z";

function wrap(node: ReactNode) {
  return <StateProvider>{node}</StateProvider>;
}

function makeTask(partial: Partial<Task> = {}): Task {
  return {
    id: "task-1",
    workspaceId: "ws-1",
    identifier: "T-1",
    title: "T",
    status: "in_progress",
    priority: "medium",
    labels: [],
    blockedBy: [],
    blocking: [],
    children: [],
    reviewers: [],
    approvers: [],
    decisions: [],
    createdBy: "u-1",
    createdAt: T_10,
    updatedAt: T_10,
    ...partial,
  };
}

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

describe("ChatActivityTabs per-agent tabs", () => {
  it("adds one tab per office agent, labeled with the agent name", () => {
    const task = makeTask();
    const sessions: TaskSession[] = [
      officeSession("s-ceo", "agent-ceo", "IDLE", T_10, "CEO"),
      officeSession("s-dev", "agent-dev", "IDLE", T_11, "Eng Lead"),
    ];
    render(
      wrap(
        <ChatActivityTabs
          task={task}
          comments={[]}
          activity={[]}
          sessions={sessions}
          scrollParent={null}
          readOnly={false}
        />,
      ),
    );
    // Tab triggers expose the agent's name in the visible label.
    expect(screen.getByRole("tab", { name: /CEO/ })).toBeTruthy();
    expect(screen.getByRole("tab", { name: /Eng Lead/ })).toBeTruthy();
    // Chat and Activity tabs are still there.
    expect(screen.getByRole("tab", { name: "Chat" })).toBeTruthy();
    expect(screen.getByRole("tab", { name: "Activity" })).toBeTruthy();
  });

  it("renders no extra tabs when there are no office sessions", () => {
    const task = makeTask();
    render(
      wrap(
        <ChatActivityTabs
          task={task}
          comments={[]}
          activity={[]}
          sessions={[]}
          scrollParent={null}
          readOnly={false}
        />,
      ),
    );
    expect(screen.queryByTestId("agent-tab-live-dot")).toBeNull();
    // Only Chat + Activity tabs.
    expect(screen.getAllByRole("tab")).toHaveLength(2);
  });

  it("shows a live indicator on the agent tab when any of the agent's sessions is RUNNING", () => {
    const task = makeTask();
    const sessions: TaskSession[] = [
      officeSession("s-ceo", "agent-ceo", "IDLE", T_10, "CEO"),
      officeSession("s-dev", "agent-dev", "RUNNING", T_11, "Eng Lead"),
    ];
    render(
      wrap(
        <ChatActivityTabs
          task={task}
          comments={[]}
          activity={[]}
          sessions={sessions}
          scrollParent={null}
          readOnly={false}
        />,
      ),
    );
    const dots = screen.getAllByTestId("agent-tab-live-dot");
    // Exactly one dot — the running agent.
    expect(dots).toHaveLength(1);
  });

  it("groups multiple sessions for one agent into a single tab", () => {
    const task = makeTask();
    const sessions: TaskSession[] = [
      officeSession("s-old", "agent-a", "IDLE", T_10, "Eng Lead"),
      officeSession("s-new", "agent-a", "RUNNING", T_12, "Eng Lead"),
    ];
    render(
      wrap(
        <ChatActivityTabs
          task={task}
          comments={[]}
          activity={[]}
          sessions={sessions}
          scrollParent={null}
          readOnly={false}
        />,
      ),
    );
    // Only one extra tab even though there are two sessions for agent-a.
    const agentTabs = screen
      .getAllByRole("tab")
      .filter((t) => /Eng Lead/.test(t.textContent ?? ""));
    expect(agentTabs).toHaveLength(1);
  });
});

describe("ChatActivityTabs agent tab content", () => {
  // We assert the wiring deterministically: the agent tab trigger's
  // aria-controls points to a Radix tab content panel keyed by
  // `agent-<group.id>`. Radix mounts the content lazily when its tab
  // becomes active — re-rendering control flow is what we care about
  // here, not the click itself.
  it("wires the agent tab trigger to a content panel keyed by the representative session id", () => {
    const task = makeTask();
    const sessions: TaskSession[] = [
      officeSession("s-old", "agent-a", "IDLE", T_10, "Eng Lead"),
      officeSession("s-new", "agent-a", "RUNNING", T_12, "Eng Lead"),
    ];
    render(
      wrap(
        <ChatActivityTabs
          task={task}
          comments={[]}
          activity={[]}
          sessions={sessions}
          scrollParent={null}
          readOnly={false}
        />,
      ),
    );
    const tab = screen.getByRole("tab", { name: /Eng Lead/ });
    // Representative session is the most recent — s-new.
    expect(tab.getAttribute("aria-controls")).toContain("agent-s-new");
  });
});

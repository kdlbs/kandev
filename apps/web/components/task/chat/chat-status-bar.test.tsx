import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { useSyncExternalStore, type ComponentProps } from "react";
import { ChatStatusBar, shouldRenderChatStatusBar } from "./chat-status-bar";

const statusStore = vi.hoisted(() => {
  const state = {
    userSettings: { showTranscriptAutoScrollControl: false },
    taskSessions: {
      items: {
        "session-1": {
          id: "session-1",
          task_id: "",
          state: "IDLE",
          metadata: {
            acp: {
              meta: {
                goal: {
                  objective: "Coordinate contributor PR reviews",
                  status: "active",
                  createdAt: 10,
                  updatedAt: 20,
                },
              },
            },
          },
        },
      },
    },
  };
  const listeners = new Set<() => void>();
  return {
    state,
    subscribe(listener: () => void) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: <T,>(selector: (state: typeof statusStore.state) => T) =>
    useSyncExternalStore(
      statusStore.subscribe,
      () => selector(statusStore.state),
      () => selector(statusStore.state),
    ),
}));

vi.mock("@/components/task/workflow-move-proceed-button", () => ({
  WorkflowMoveProceedButton: () => null,
}));

vi.mock("@/components/github/pr-status-chip", () => ({ PRStatusChip: () => null }));
vi.mock("@/components/gitlab/mr-status-chip", () => ({ MRStatusChip: () => null }));
vi.mock("@/components/task/task-dependency-chip", () => ({ TaskDependencyChip: () => null }));
vi.mock("@/components/azure-devops/azure-devops-task-pull-request-chip", () => ({
  AzureDevOpsTaskPullRequestChip: () => null,
}));
vi.mock("@/components/integrations/registered-change-request-status", () => ({
  RegisteredChangeRequestStatus: () => <span data-testid="registered-review-status" />,
}));
vi.mock("@/components/task/share/share-button", () => ({
  shareableSessionStateClient: () => false,
}));
vi.mock("@/components/task/chat/transcript-nav-group", () => ({
  TranscriptNavGroup: () => null,
}));
vi.mock("@/components/threads/open-in-threads-button", () => ({
  OpenInThreadsButton: () => null,
}));
vi.mock("@/hooks/domains/threads/use-deck-thread", () => ({ useIsDeckThread: () => false }));
vi.mock("./todo-indicator", () => ({ TodoIndicator: () => null }));
vi.mock("./auto-scroll-toggle-button", () => ({ AutoScrollToggleButton: () => null }));
vi.mock("./pr-archive-banners", () => ({
  PRMergedBanner: () => null,
  PRClosedBanner: () => null,
}));
vi.mock("./task-autopilot-chat-chip", () => ({
  AutopilotChatChip: () => null,
  useTaskAutopilot: () => false,
}));
vi.mock("./agent-goal-chip", () => ({
  AgentGoalChip: ({ goal }: { goal: { objective: string } | null }) =>
    goal ? <span data-testid="agent-goal-chip">{goal.objective}</span> : null,
}));

afterEach(cleanup);

function renderStatus(overrides: Partial<ComponentProps<typeof ChatStatusBar>> = {}) {
  return render(
    <ChatStatusBar
      todoItems={[]}
      taskId={null}
      sessionId="session-1"
      sessionState="IDLE"
      nextStepName={null}
      onProceed={vi.fn()}
      isAgentBusy={false}
      hasPendingClarification={false}
      isMoving={false}
      {...overrides}
    />,
  );
}

describe("chat status bar goal visibility", () => {
  it("renders when the active goal is the only status item", () => {
    renderStatus();

    expect(screen.getByTestId("chat-status-bar")).toBeTruthy();
    expect(screen.getByTestId("agent-goal-chip").textContent).toContain(
      "Coordinate contributor PR reviews",
    );
  });

  it("places the goal after review status and before the queue chip", () => {
    renderStatus({ queueChip: <span data-testid="queue-chip" /> });

    const row = screen.getByTestId("chat-status-bar");
    const order = Array.from(row.children).map((child) => child.getAttribute("data-testid"));
    expect(order.indexOf("registered-review-status")).toBeLessThan(
      order.indexOf("agent-goal-chip"),
    );
    expect(order.indexOf("agent-goal-chip")).toBeLessThan(order.indexOf("queue-chip"));
  });

  it("counts an active goal when deciding whether the row is needed", () => {
    expect(
      shouldRenderChatStatusBar({
        hasTask: false,
        hasTodos: false,
        hasQueueChip: false,
        showRightControls: false,
        showProceed: false,
        hasGoal: true,
      }),
    ).toBe(true);
  });
});

import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { useSyncExternalStore, type ComponentProps } from "react";
import { SelectionActionSlotProvider } from "@/components/shared/selection-action-slot";
import { ChatStatusBar, shouldRenderChatStatusBar } from "./chat-status-bar";

const USAGE_TRIGGER_TEST_ID = "conversation-usage-trigger";
const GRAPH_BUTTON_TEST_ID = "task-graph-button";
const RIGHT_CONTROLS_TEST_ID = "chat-status-bar-right-controls";
const SHARE_BUTTON_TEST_ID = "share-task-button";
const PROCEED_TEST_ID = "proceed-next-step";

const statusStore = vi.hoisted(() => {
  const state = {
    features: { agentBackgroundWork: false },
    userSettings: { showTranscriptAutoScrollControl: false },
    taskSessions: {
      items: {
        "session-1": {
          id: "session-1",
          task_id: "",
          state: "IDLE",
          agent_profile_id: undefined as string | undefined,
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
    agentProfiles: {
      items: [
        {
          id: "profile-antigravity",
          label: "Antigravity • Personal",
          agent_id: "agent-antigravity",
          agent_name: "antigravity-acp",
          cli_passthrough: false,
        },
      ],
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
const usageDisplay = vi.hoisted(() => ({ visible: true }));
const viewport = vi.hoisted(() => ({ isMobile: false }));
const shareable = vi.hoisted(() => ({ value: false }));
const graphAvailable = vi.hoisted(() => ({ value: false }));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    breakpoint: viewport.isMobile ? "mobile" : "desktop",
    isMobile: viewport.isMobile,
    isTablet: false,
    isDesktop: !viewport.isMobile,
    isCompactDesktop: false,
    isFullDesktop: !viewport.isMobile,
    isFinePointer: true,
    usesDesktopWorkbench: !viewport.isMobile,
  }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: <T,>(selector: (state: typeof statusStore.state) => T) =>
    useSyncExternalStore(
      statusStore.subscribe,
      () => selector(statusStore.state),
      () => selector(statusStore.state),
    ),
  useAppStoreApi: () => ({ getState: () => statusStore.state }),
}));

vi.mock("@/components/task/workflow-move-proceed-button", () => ({
  WorkflowMoveProceedButton: ({ className, testId }: { className?: string; testId?: string }) => (
    <button className={className} data-testid={testId} />
  ),
}));

vi.mock("@/components/github/pr-status-chip", () => ({ PRStatusChip: () => null }));
vi.mock("@/components/gitlab/mr-status-chip", () => ({ MRStatusChip: () => null }));
vi.mock("@/components/task/task-dependency-chip", () => ({ TaskDependencyChip: () => null }));
vi.mock("@/components/task-graph/task-graph-button", () => ({
  TaskGraphButton: ({ taskId }: { taskId: string | null }) =>
    graphAvailable.value && taskId ? (
      <button type="button" data-testid={GRAPH_BUTTON_TEST_ID} data-task-id={taskId} />
    ) : null,
}));
vi.mock("@/components/azure-devops/azure-devops-task-pull-request-chip", () => ({
  AzureDevOpsTaskPullRequestChip: () => null,
}));
vi.mock("@/components/integrations/registered-change-request-status", () => ({
  RegisteredChangeRequestStatus: () => <span data-testid="registered-review-status" />,
}));
vi.mock("@/components/task/share/share-button", () => ({
  shareableSessionStateClient: () => shareable.value,
}));
vi.mock("@/components/task/chat/transcript-nav-group", () => ({
  TranscriptNavGroup: ({
    usageControl,
    canShare,
  }: {
    usageControl?: React.ReactNode;
    canShare?: boolean;
  }) => (
    <>
      {usageControl}
      {canShare ? <button type="button" data-testid={SHARE_BUTTON_TEST_ID} /> : null}
    </>
  ),
}));
vi.mock("./conversation-usage-display", () => ({
  ConversationUsageDisplay: ({
    taskId,
    sessionId,
    subscriptionBilling,
  }: {
    taskId: string;
    sessionId: string;
    subscriptionBilling?: boolean;
  }) =>
    usageDisplay.visible ? (
      <button
        type="button"
        aria-label="Usage"
        data-testid="conversation-usage-trigger"
        data-task-id={taskId}
        data-session-id={sessionId}
        data-subscription-billing={subscriptionBilling ? "true" : "false"}
      />
    ) : null,
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

afterEach(() => {
  cleanup();
  usageDisplay.visible = true;
  viewport.isMobile = false;
  shareable.value = false;
  graphAvailable.value = false;
});

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

    const review = screen.getByTestId("registered-review-status");
    const goal = screen.getByTestId("agent-goal-chip");
    const queue = screen.getByTestId("queue-chip");
    expect(review.compareDocumentPosition(goal) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(goal.compareDocumentPosition(queue) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
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

describe("conversation usage status control", () => {
  // @covers AC-UI-COMPOSER-ACTION-WRAP-001.2, .4, .5, .6, .7
  it("preserves status, action, and utility instances when responsive order changes", () => {
    const props: ComponentProps<typeof ChatStatusBar> = {
      todoItems: [],
      taskId: "task-1",
      sessionId: "session-1",
      sessionState: "IDLE",
      nextStepName: "Review",
      onProceed: vi.fn(),
      isAgentBusy: false,
      hasPendingClarification: false,
      isMoving: false,
    };
    const { rerender } = render(<ChatStatusBar {...props} />);
    const proceed = screen.getByTestId(PROCEED_TEST_ID);
    const usage = screen.getByTestId(USAGE_TRIGGER_TEST_ID);
    const review = screen.getByTestId("registered-review-status");
    for (const isMobile of [true, false, true]) {
      viewport.isMobile = isMobile;
      rerender(<ChatStatusBar {...props} />);
      expect(screen.getByTestId(PROCEED_TEST_ID)).toBe(proceed);
      expect(screen.getByTestId(USAGE_TRIGGER_TEST_ID)).toBe(usage);
      expect(screen.getByTestId("registered-review-status")).toBe(review);
      const order = proceed.compareDocumentPosition(usage);
      expect(Boolean(order & Node.DOCUMENT_POSITION_FOLLOWING)).toBe(isMobile);
      expect(
        Boolean(proceed.compareDocumentPosition(review) & Node.DOCUMENT_POSITION_FOLLOWING),
      ).toBe(isMobile);
    }
  });

  it("mounts Usage in the status row for the selected task session", () => {
    renderStatus({ taskId: "task-1", sessionId: "session-1" });

    const row = screen.getByTestId("chat-status-bar");
    const trigger = screen.getByTestId(USAGE_TRIGGER_TEST_ID);
    expect(row.contains(trigger)).toBe(true);
    expect(trigger.getAttribute("data-task-id")).toBe("task-1");
    expect(trigger.getAttribute("data-session-id")).toBe("session-1");
  });

  it("leaves no right-control spacer when Usage has no content", () => {
    usageDisplay.visible = false;
    renderStatus({ taskId: "task-1", sessionId: "session-1" });

    const controls = screen.getByTestId(RIGHT_CONTROLS_TEST_ID);
    expect(controls.childElementCount).toBe(0);
    expect(controls.className).toContain("empty:hidden");
  });

  it("keeps the proceed action right-aligned when Usage has no content", () => {
    usageDisplay.visible = false;
    renderStatus({
      taskId: "task-1",
      sessionId: "session-1",
      nextStepName: "Review",
    });

    const actions = screen.getByTestId("chat-status-bar-actions");
    const proceed = screen.getByTestId(PROCEED_TEST_ID);
    expect(actions.className).toContain("ml-auto");
    expect(actions.contains(proceed)).toBe(true);
    expect(proceed.className).toContain("h-6");
  });

  // @covers AC-UI-COMPOSER-ACTION-WRAP-001.2
  it("keeps the phone next-step action inline with the utility controls", () => {
    viewport.isMobile = true;
    renderStatus({ taskId: "task-1", sessionId: "session-1", nextStepName: "Done" });

    const proceed = screen.getByTestId(PROCEED_TEST_ID);
    expect(proceed.parentElement?.className).not.toContain("w-full");
    expect(proceed.className).not.toContain("max-md:ml-auto");
  });

  it("ends the utility controls with the selection action slot", () => {
    render(
      <SelectionActionSlotProvider>
        <ChatStatusBar
          todoItems={[]}
          taskId="task-1"
          sessionId="session-1"
          sessionState="IDLE"
          nextStepName={null}
          onProceed={vi.fn()}
          isAgentBusy={false}
          hasPendingClarification={false}
          isMoving={false}
        />
      </SelectionActionSlotProvider>,
    );

    const controls = screen.getByTestId(RIGHT_CONTROLS_TEST_ID);
    expect(controls.lastElementChild).toBe(screen.getByTestId("selection-action-slot"));
  });

  it("marks Antigravity subscription sessions for the cost disclosure", () => {
    const session = statusStore.state.taskSessions.items["session-1"];
    session.agent_profile_id = "profile-antigravity";
    try {
      renderStatus({ taskId: "task-1", sessionId: "session-1" });

      expect(
        screen.getByTestId(USAGE_TRIGGER_TEST_ID).getAttribute("data-subscription-billing"),
      ).toBe("true");
    } finally {
      session.agent_profile_id = undefined;
    }
  });
});

describe("task graph composer action", () => {
  // @covers AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.6, .9
  it("shows one graph control after Share in the right utility group", () => {
    graphAvailable.value = true;
    shareable.value = true;
    renderStatus({ taskId: "task-1", sessionId: "session-1" });

    const controls = screen.getByTestId(RIGHT_CONTROLS_TEST_ID);
    const graph = screen.getByTestId(GRAPH_BUTTON_TEST_ID);
    const share = screen.getByTestId(SHARE_BUTTON_TEST_ID);
    expect(screen.getAllByTestId(GRAPH_BUTTON_TEST_ID)).toHaveLength(1);
    expect(controls.contains(graph)).toBe(true);
    expect(graph.getAttribute("data-task-id")).toBe("task-1");
    expect(share.compareDocumentPosition(graph) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  // @covers AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.9
  it("keeps the graph action for a qualifying task without a session", () => {
    graphAvailable.value = true;
    renderStatus({ taskId: "task-1", sessionId: null, sessionState: null });

    const controls = screen.getByTestId(RIGHT_CONTROLS_TEST_ID);
    const graph = screen.getByTestId(GRAPH_BUTTON_TEST_ID);
    expect(controls.contains(graph)).toBe(true);
    expect(screen.queryByTestId(USAGE_TRIGGER_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(SHARE_BUTTON_TEST_ID)).toBeNull();
  });

  it("hides the graph action when the task has no relations", () => {
    renderStatus({ taskId: "task-1", sessionId: "session-1" });

    expect(screen.queryByTestId(GRAPH_BUTTON_TEST_ID)).toBeNull();
  });

  // @covers AC-TASKS-TASK-RELATIONSHIP-GRAPH-001.6, .8
  it("keeps one graph instance in the right group across responsive order changes", () => {
    graphAvailable.value = true;
    const props: ComponentProps<typeof ChatStatusBar> = {
      todoItems: [],
      taskId: "task-1",
      sessionId: "session-1",
      sessionState: "IDLE",
      nextStepName: "Review",
      onProceed: vi.fn(),
      isAgentBusy: false,
      hasPendingClarification: false,
      isMoving: false,
    };
    const { rerender } = render(<ChatStatusBar {...props} />);
    const graph = screen.getByTestId(GRAPH_BUTTON_TEST_ID);
    for (const isMobile of [true, false, true]) {
      viewport.isMobile = isMobile;
      rerender(<ChatStatusBar {...props} />);
      expect(screen.getAllByTestId(GRAPH_BUTTON_TEST_ID)).toHaveLength(1);
      expect(screen.getByTestId(GRAPH_BUTTON_TEST_ID)).toBe(graph);
      expect(screen.getByTestId(RIGHT_CONTROLS_TEST_ID).contains(graph)).toBe(true);
    }
  });
});

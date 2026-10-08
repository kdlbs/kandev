import { useEffect } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { ConversationResponse } from "@/lib/api/domains/coordinator-api";
import type { OpenSequenceState } from "@/hooks/domains/coordinator/use-copilot-open-sequence";
import type { CopilotChip } from "@/hooks/domains/coordinator/copilot-store";

const useCoordinatorCopilot = vi.hoisted(() => vi.fn());
const messagesBySession = vi.hoisted(() => ({}) as Record<string, unknown[]>);
const taskSessionItems = vi.hoisted(() => ({}) as Record<string, Record<string, unknown>>);
const quickChatSessionViewCalls = vi.hoisted(() => [] as Array<Record<string, unknown>>);
const quickChatSessionViewMounts = vi.hoisted(() => [] as Array<Record<string, unknown>>);

vi.mock("./use-coordinator-copilot", () => ({ useCoordinatorCopilot }));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      messages: { bySession: messagesBySession },
      taskSessions: { items: taskSessionItems },
    }),
}));

vi.mock("@/components/quick-chat/quick-chat-session-view", () => ({
  QuickChatSessionView: (props: Record<string, unknown>) => {
    quickChatSessionViewCalls.push(props);
    // eslint-disable-next-line react-hooks/rules-of-hooks -- fixed mock component, not a conditional hook call.
    useEffect(() => {
      quickChatSessionViewMounts.push(props.session as Record<string, unknown>);
      // eslint-disable-next-line react-hooks/exhaustive-deps -- mount-only: records once per remount (a key change), not per prop update.
    }, []);
    return (
      <div data-testid="quick-chat-session-view-marker" data-task-id={String(props.session)} />
    );
  },
}));

import { CoordinatorCopilot } from "./coordinator-copilot";

const WORKSPACE_ID = "ws-1";
const COORDINATOR_ID = "coord-1";
const COORDINATOR_NAME = "Backend coordinator";

const conversation: ConversationResponse = {
  task_id: "task-1",
  session_id: "session-1",
  archive_state: false,
};

const chip: CopilotChip = {
  id: "KAN-418",
  label: "KAN-418",
  ref: { kind: "task", id: "task-418" },
};
const SUGGESTION_QUESTION = "What needs me first, and why?";
const QUICK_CHAT_MARKER_TEST_ID = "quick-chat-session-view-marker";

function mockController(overrides: Partial<ReturnType<typeof useCoordinatorCopilot>> = {}) {
  useCoordinatorCopilot.mockReturnValue({
    enabled: true,
    open: false,
    launcher: { coordinator: null, loading: false, busy: false, gone: false },
    openSequence: { state: { kind: "idle" } as OpenSequenceState, open: vi.fn(), retry: vi.fn() },
    routeSession: null,
    chip: null,
    pendingDraft: undefined,
    askKey: 0,
    handleOpenChange: vi.fn(),
    removeChip: vi.fn(),
    suggest: vi.fn(),
    ...overrides,
  });
}

function renderCopilot() {
  return render(
    <TooltipProvider delayDuration={0}>
      <CoordinatorCopilot
        workspaceId={WORKSPACE_ID}
        coordinatorId={COORDINATOR_ID}
        coordinatorName={COORDINATOR_NAME}
        canManage
      >
        <div data-testid="screen" />
      </CoordinatorCopilot>
    </TooltipProvider>,
  );
}

function mockReadyEndedSession(overrides: Partial<ReturnType<typeof useCoordinatorCopilot>> = {}) {
  mockController({
    open: true,
    openSequence: {
      state: { kind: "ready", session: conversation },
      open: vi.fn(),
      retry: vi.fn(),
    },
    routeSession: conversation,
    ...overrides,
  });
}

beforeEach(() => {
  mockController();
});

afterEach(() => {
  cleanup();
  quickChatSessionViewCalls.length = 0;
  quickChatSessionViewMounts.length = 0;
  for (const key of Object.keys(messagesBySession)) delete messagesBySession[key];
  for (const key of Object.keys(taskSessionItems)) delete taskSessionItems[key];
  vi.clearAllMocks();
});

describe("CoordinatorCopilot", () => {
  it("renders nothing when disabled", () => {
    mockController({ enabled: false });
    renderCopilot();
    expect(screen.getByTestId("screen")).toBeTruthy();
    expect(screen.queryByTestId("coordinator-copilot-launcher")).toBeNull();
    expect(screen.queryByTestId("coordinator-copilot-popover")).toBeNull();
  });

  it("renders the launcher with an idle accessible name", () => {
    renderCopilot();
    expect(screen.getByRole("button", { name: `Chat with ${COORDINATOR_NAME}` })).toBeTruthy();
  });

  it("renders the launcher with a distinct busy accessible name", () => {
    mockController({ launcher: { coordinator: null, loading: false, busy: true, gone: false } });
    renderCopilot();
    expect(screen.getByRole("button", { name: `${COORDINATOR_NAME} is working` })).toBeTruthy();
  });

  it("shows the profile-unavailable messages and no composer", async () => {
    mockController({
      open: true,
      openSequence: {
        state: { kind: "profile-unavailable", agentStatus: "missing", executorStatus: "ok" },
        open: vi.fn(),
        retry: vi.fn(),
      },
    });
    renderCopilot();
    await waitFor(() => screen.getByTestId("copilot-profile-messages"));
    expect(
      screen.getByText("The agent profile was removed. Choose another in Settings."),
    ).toBeTruthy();
    expect(screen.queryByTestId(QUICK_CHAT_MARKER_TEST_ID)).toBeNull();
  });

  it("shows both profile messages when neither status is ok", async () => {
    mockController({
      open: true,
      openSequence: {
        state: {
          kind: "profile-unavailable",
          agentStatus: "passthrough",
          executorStatus: "missing",
        },
        open: vi.fn(),
        retry: vi.fn(),
      },
    });
    renderCopilot();
    await waitFor(() => screen.getByTestId("copilot-profile-messages"));
    expect(
      screen.getByText(
        "The agent profile uses CLI passthrough, which a coordinator cannot use. Choose another in Settings.",
      ),
    ).toBeTruthy();
    expect(screen.getByText("The executor was removed. Choose another in Settings.")).toBeTruthy();
  });

  it("shows the gone message with no Try again", async () => {
    mockController({
      open: true,
      openSequence: { state: { kind: "gone" }, open: vi.fn(), retry: vi.fn() },
    });
    renderCopilot();
    await waitFor(() => screen.getByTestId("copilot-gone-message"));
    expect(screen.queryByRole("button", { name: "Try again" })).toBeNull();
  });

  it("shows a retryable error and calls retry on Try again", async () => {
    const retry = vi.fn();
    mockController({
      open: true,
      openSequence: { state: { kind: "error", error: "open-failed" }, open: vi.fn(), retry },
    });
    renderCopilot();
    await waitFor(() => screen.getByText("Could not open the conversation."));
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(retry).toHaveBeenCalledTimes(1);
  });
});

describe("CoordinatorCopilot - ready conversation", () => {
  it("renders QuickChatSessionView with the route session's taskId once ready", async () => {
    mockController({
      open: true,
      openSequence: {
        state: { kind: "ready", session: conversation },
        open: vi.fn(),
        retry: vi.fn(),
      },
      routeSession: conversation,
    });
    renderCopilot();
    await waitFor(() => screen.getByTestId(QUICK_CHAT_MARKER_TEST_ID));
    expect(quickChatSessionViewCalls[0]).toMatchObject({
      session: {
        kind: "chat",
        sessionId: "session-1",
        workspaceId: WORKSPACE_ID,
        taskId: "task-1",
      },
      automaticRecovery: false,
      hideSessionSelectors: true,
      activityDisplay: true,
      taskArchiveState: false,
    });
  });

  it("renders the chip and calls removeChip from its remove button", async () => {
    const removeChip = vi.fn();
    messagesBySession["session-1"] = [{ id: "m1" }];
    mockController({
      open: true,
      openSequence: {
        state: { kind: "ready", session: conversation },
        open: vi.fn(),
        retry: vi.fn(),
      },
      routeSession: conversation,
      chip,
      removeChip,
    });
    renderCopilot();
    await waitFor(() => screen.getByText("about KAN-418"));
    fireEvent.click(screen.getByRole("button", { name: "Remove" }));
    expect(removeChip).toHaveBeenCalledTimes(1);
  });

  it("shows the empty-conversation suggestion only while the transcript is empty", async () => {
    const suggest = vi.fn();
    mockController({
      open: true,
      openSequence: {
        state: { kind: "ready", session: conversation },
        open: vi.fn(),
        retry: vi.fn(),
      },
      routeSession: conversation,
      suggest,
    });
    renderCopilot();
    await waitFor(() => screen.getByText(SUGGESTION_QUESTION));
    fireEvent.click(screen.getByRole("button", { name: SUGGESTION_QUESTION }));
    expect(suggest).toHaveBeenCalledWith(SUGGESTION_QUESTION);
  });

  it("hides the empty-conversation suggestion once the transcript has a message", async () => {
    messagesBySession["session-1"] = [{ id: "m1" }];
    mockController({
      open: true,
      openSequence: {
        state: { kind: "ready", session: conversation },
        open: vi.fn(),
        retry: vi.fn(),
      },
      routeSession: conversation,
    });
    renderCopilot();
    await waitFor(() => screen.getByTestId(QUICK_CHAT_MARKER_TEST_ID));
    expect(screen.queryByText(SUGGESTION_QUESTION)).toBeNull();
  });
});

describe("CoordinatorCopilot - ready conversation: revalidation in flight", () => {
  it("keeps the composer mounted while a held route session revalidates (loading)", async () => {
    mockController({
      open: true,
      openSequence: { state: { kind: "loading" }, open: vi.fn(), retry: vi.fn() },
      routeSession: conversation,
    });
    renderCopilot();
    await waitFor(() => screen.getByTestId(QUICK_CHAT_MARKER_TEST_ID));
    expect(quickChatSessionViewCalls[0]).toMatchObject({
      session: { kind: "chat", sessionId: "session-1" },
    });
  });

  it("removing the chip does not blank the composer while the route session is held", async () => {
    const removeChip = vi.fn();
    mockController({
      open: true,
      openSequence: { state: { kind: "loading" }, open: vi.fn(), retry: vi.fn() },
      routeSession: conversation,
      chip,
      removeChip,
    });
    renderCopilot();
    await waitFor(() => screen.getByText("about KAN-418"));
    fireEvent.click(screen.getByRole("button", { name: "Remove" }));
    expect(removeChip).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId(QUICK_CHAT_MARKER_TEST_ID)).toBeTruthy();
  });
});

describe("CoordinatorCopilot - ready conversation: transformOutgoing", () => {
  it("passes a transformOutgoing that prefixes the chip id while a chip is set", async () => {
    mockController({
      open: true,
      openSequence: {
        state: { kind: "ready", session: conversation },
        open: vi.fn(),
        retry: vi.fn(),
      },
      routeSession: conversation,
      chip,
    });
    renderCopilot();
    await waitFor(() => screen.getByTestId(QUICK_CHAT_MARKER_TEST_ID));
    const transformOutgoing = quickChatSessionViewCalls[0].transformOutgoing as (
      message: string,
    ) => string;
    expect(transformOutgoing("Why is this here?")).toBe(
      "About KAN-418 [task:task-418]: Why is this here?",
    );
  });

  it.each([
    [
      { id: "wf-1", label: "Sprint: board\nA", ref: { kind: "workflow", id: "wf-1" } },
      "About Sprint - board A [workflow:wf-1]: hi",
    ],
    [
      { id: "task-7", label: "KAN-7", ref: { kind: "task", id: "task-7" } },
      "About KAN-7 [task:task-7]: hi",
    ],
  ])("prefixes the label and the reference of chip %#", async (workspaceChip, expected) => {
    mockController({
      open: true,
      openSequence: {
        state: { kind: "ready", session: conversation },
        open: vi.fn(),
        retry: vi.fn(),
      },
      routeSession: conversation,
      chip: workspaceChip as unknown as typeof chip,
    });
    renderCopilot();
    await waitFor(() => screen.getByTestId(QUICK_CHAT_MARKER_TEST_ID));
    const transformOutgoing = quickChatSessionViewCalls[0].transformOutgoing as (
      message: string,
    ) => string;
    expect(transformOutgoing("hi")).toBe(expected);
  });

  it("passes no transformOutgoing when no chip is set", async () => {
    mockController({
      open: true,
      openSequence: {
        state: { kind: "ready", session: conversation },
        open: vi.fn(),
        retry: vi.fn(),
      },
      routeSession: conversation,
      chip: null,
    });
    renderCopilot();
    await waitFor(() => screen.getByTestId(QUICK_CHAT_MARKER_TEST_ID));
    expect(quickChatSessionViewCalls[0].transformOutgoing).toBeUndefined();
  });
});

const SESSION_RECOVERY_TEST_ID = "session-recovery-error";

describe("CoordinatorCopilot - ended session banner", () => {
  const COULDNT_START_TITLE = "Couldn't start a session";
  const FALLBACK_DETAIL = "The backend rejected the session request.";

  it.each(["FAILED", "CANCELLED", "COMPLETED"])(
    "shows the recovery banner with the session's error_message for %s",
    async (state) => {
      taskSessionItems["session-1"] = { state, error_message: "agent process exited: boom" };
      mockReadyEndedSession();
      renderCopilot();
      await waitFor(() => screen.getByTestId(SESSION_RECOVERY_TEST_ID));
      expect(screen.getByText(COULDNT_START_TITLE)).toBeTruthy();
      expect(screen.getByText("agent process exited: boom")).toBeTruthy();
    },
  );

  it.each(["FAILED", "CANCELLED", "COMPLETED"])(
    "falls back to the shared copy when %s carries no error_message",
    async (state) => {
      taskSessionItems["session-1"] = { state, error_message: "" };
      mockReadyEndedSession();
      renderCopilot();
      await waitFor(() => screen.getByTestId(SESSION_RECOVERY_TEST_ID));
      expect(screen.getAllByText(FALLBACK_DETAIL).length).toBeGreaterThan(0);
    },
  );

  it.each(["CREATED", "RUNNING", "WAITING_FOR_INPUT"])(
    "shows no banner while the session is %s",
    async (state) => {
      taskSessionItems["session-1"] = { state };
      mockReadyEndedSession();
      renderCopilot();
      await waitFor(() => screen.getByTestId(QUICK_CHAT_MARKER_TEST_ID));
      expect(screen.queryByTestId(SESSION_RECOVERY_TEST_ID)).toBeNull();
    },
  );

  it("shows no banner when the session row is missing from the store", async () => {
    mockReadyEndedSession();
    renderCopilot();
    await waitFor(() => screen.getByTestId(QUICK_CHAT_MARKER_TEST_ID));
    expect(screen.queryByTestId(SESSION_RECOVERY_TEST_ID)).toBeNull();
  });

  it("hides the empty-conversation intro while the session has ended", async () => {
    taskSessionItems["session-1"] = { state: "FAILED", error_message: "boom" };
    mockReadyEndedSession();
    renderCopilot();
    await waitFor(() => screen.getByTestId(SESSION_RECOVERY_TEST_ID));
    expect(screen.queryByText(SUGGESTION_QUESTION)).toBeNull();
  });

  it("links to the popover's workspace settings when the session ended for a missing agent profile", async () => {
    taskSessionItems["session-1"] = {
      state: "FAILED",
      error_message: "agent_profile_id is required",
    };
    mockReadyEndedSession();
    renderCopilot();
    await waitFor(() => screen.getByTestId(SESSION_RECOVERY_TEST_ID));
    const action = screen.getByTestId("ensure-session-error-action");
    expect(action.getAttribute("href")).toBe(`/settings/workspaces/${WORKSPACE_ID}`);
  });
});

describe("CoordinatorCopilot - ended session retry", () => {
  it("Retry calls the open sequence's retry once and is disabled while loading", async () => {
    const retry = vi.fn();
    taskSessionItems["session-1"] = { state: "FAILED", error_message: "boom" };
    mockReadyEndedSession({ openSequence: { state: { kind: "loading" }, open: vi.fn(), retry } });
    renderCopilot();
    await waitFor(() => screen.getByTestId(SESSION_RECOVERY_TEST_ID));
    const retryButton = screen.getByTestId("ensure-session-error-retry");
    expect((retryButton as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(retryButton);
    expect(retry).not.toHaveBeenCalled();
  });

  it("Retry calls the open sequence's retry exactly once when enabled", async () => {
    const retry = vi.fn();
    taskSessionItems["session-1"] = { state: "FAILED", error_message: "boom" };
    mockReadyEndedSession({
      openSequence: { state: { kind: "ready", session: conversation }, open: vi.fn(), retry },
    });
    renderCopilot();
    await waitFor(() => screen.getByTestId(SESSION_RECOVERY_TEST_ID));
    fireEvent.click(screen.getByTestId("ensure-session-error-retry"));
    expect(retry).toHaveBeenCalledTimes(1);
  });

  it("a Retry that returns a fresh session removes the banner and remounts the view", async () => {
    taskSessionItems["session-1"] = { state: "FAILED", error_message: "boom" };
    mockReadyEndedSession();
    const { rerender } = renderCopilot();
    await waitFor(() => screen.getByTestId(SESSION_RECOVERY_TEST_ID));
    expect(quickChatSessionViewMounts).toHaveLength(1);

    const freshConversation: ConversationResponse = {
      task_id: "task-2",
      session_id: "session-2",
      archive_state: false,
    };
    mockReadyEndedSession({
      openSequence: {
        state: { kind: "ready", session: freshConversation },
        open: vi.fn(),
        retry: vi.fn(),
      },
      routeSession: freshConversation,
    });
    rerender(
      <TooltipProvider delayDuration={0}>
        <CoordinatorCopilot
          workspaceId={WORKSPACE_ID}
          coordinatorId={COORDINATOR_ID}
          coordinatorName={COORDINATOR_NAME}
          canManage
        >
          <div data-testid="screen" />
        </CoordinatorCopilot>
      </TooltipProvider>,
    );
    await waitFor(() => expect(screen.queryByTestId(SESSION_RECOVERY_TEST_ID)).toBeNull());
    expect(quickChatSessionViewMounts).toHaveLength(2);
    expect(screen.getByText(SUGGESTION_QUESTION)).toBeTruthy();
  });
});

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { ConversationResponse } from "@/lib/api/domains/coordinator-api";
import type { OpenSequenceState } from "@/hooks/domains/coordinator/use-copilot-open-sequence";
import type { CopilotChip } from "@/hooks/domains/coordinator/copilot-store";

const useCoordinatorCopilot = vi.hoisted(() => vi.fn());
const messagesBySession = vi.hoisted(() => ({}) as Record<string, unknown[]>);
const quickChatSessionViewCalls = vi.hoisted(() => [] as Array<Record<string, unknown>>);

vi.mock("./use-coordinator-copilot", () => ({ useCoordinatorCopilot }));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ messages: { bySession: messagesBySession } }),
}));

vi.mock("@/components/quick-chat/quick-chat-session-view", () => ({
  QuickChatSessionView: (props: Record<string, unknown>) => {
    quickChatSessionViewCalls.push(props);
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

const chip: CopilotChip = { id: "KAN-418", label: "KAN-418" };
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
      />
    </TooltipProvider>,
  );
}

beforeEach(() => {
  mockController();
});

afterEach(() => {
  cleanup();
  quickChatSessionViewCalls.length = 0;
  for (const key of Object.keys(messagesBySession)) delete messagesBySession[key];
  vi.clearAllMocks();
});

describe("CoordinatorCopilot", () => {
  it("renders nothing when disabled", () => {
    mockController({ enabled: false });
    const { container } = renderCopilot();
    expect(container.firstChild).toBeNull();
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
    expect(transformOutgoing("Why is this here?")).toBe("About KAN-418: Why is this here?");
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

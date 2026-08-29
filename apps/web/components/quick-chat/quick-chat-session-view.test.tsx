import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const sessionRows = vi.hoisted(
  () =>
    ({}) as Record<
      string,
      { task_id?: string; agent_profile_id?: string; is_passthrough?: boolean }
    >,
);
const useEnsureTaskSession = vi.hoisted(() => vi.fn());
const useSessionResumption = vi.hoisted(() =>
  vi.fn(() => ({
    resumptionState: "idle",
    sessionStatus: null,
    error: null,
    taskSessionState: null,
    worktreePath: null,
    worktreeBranch: null,
    resumeSession: vi.fn(),
  })),
);

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      taskSessions: { items: sessionRows },
      quickChat: { sessions: [] },
      agentProfiles: { items: [] },
    }),
}));

vi.mock("@/hooks/use-ensure-task-session", () => ({ useEnsureTaskSession }));
vi.mock("@/hooks/domains/session/use-session-resumption", () => ({ useSessionResumption }));
vi.mock("@/components/task/passthrough-terminal", () => ({
  PassthroughTerminal: () => <div data-testid="passthrough-terminal" />,
}));
vi.mock("./quick-chat-content", () => ({
  QuickChatContent: () => <div data-testid="quick-chat-content" />,
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

import { QuickChatSessionView } from "./quick-chat-session-view";

const session = {
  kind: "chat" as const,
  sessionId: "session-1",
  workspaceId: "workspace-1",
};

afterEach(() => {
  cleanup();
  delete sessionRows[session.sessionId];
  vi.clearAllMocks();
});

// @covers AC-TASKS-QUICK-CHAT-EXPIRATION-001.2
describe("QuickChatSessionView session resumption", () => {
  it("mounts resumption with the task id stored on the Quick Chat descriptor", () => {
    render(<QuickChatSessionView session={{ ...session, taskId: "task-from-descriptor" }} />);

    expect(useSessionResumption).toHaveBeenCalledWith("task-from-descriptor", session.sessionId);
  });

  it("falls back to the hydrated task-session row when the descriptor has no task id", () => {
    sessionRows[session.sessionId] = { task_id: "task-from-hydrated-row" };

    render(<QuickChatSessionView session={session} />);

    expect(useSessionResumption).toHaveBeenCalledWith("task-from-hydrated-row", session.sessionId);
  });

  it("passes a null task id until session hydration provides one", () => {
    const view = render(<QuickChatSessionView session={session} />);

    expect(useSessionResumption).toHaveBeenLastCalledWith(null, session.sessionId);

    sessionRows[session.sessionId] = { task_id: "task-after-hydration" };
    view.rerender(<QuickChatSessionView session={session} />);

    expect(useSessionResumption).toHaveBeenLastCalledWith(
      "task-after-hydration",
      session.sessionId,
    );
  });
});

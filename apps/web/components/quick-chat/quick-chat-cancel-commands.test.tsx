import { cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useEffect } from "react";
import { CommandRegistryProvider, useCommands } from "@/lib/commands/command-registry";
import type { CommandItem } from "@/lib/commands/types";

const mocks = vi.hoisted(() => {
  const state = {
    quickChat: {
      isOpen: true,
      activeKind: "conversation" as "conversation" | "terminal" | "setup",
      activeSessionId: "quick-session",
    },
    tasks: { activeTaskId: "task-1", activeSessionId: "task-session" },
    kanban: { tasks: [{ id: "task-1", title: "Underlying task" }] },
    taskSessions: {
      items: {
        "quick-session": { cancellation_pending: false },
        "quick-session-b": { cancellation_pending: false },
        "task-session": { cancellation_pending: false },
      },
    },
    chatInput: { cancellingBySessionId: {} as Record<string, boolean> },
  };

  return {
    state,
    underlyingCancel: vi.fn(async () => {}),
    setCancelTurnPending: (sessionId: string, pending: boolean) => {
      if (pending) state.chatInput.cancellingBySessionId[sessionId] = true;
      else delete state.chatInput.cancellingBySessionId[sessionId];
    },
  };
});

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@/hooks/use-git-operations", () => ({
  useGitOperations: () => ({}),
}));

vi.mock("@/hooks/use-git-with-feedback", () => ({
  gitOperationLabel: (t: (key: string) => string, key: string) => t(key),
  useGitWithFeedback: () => vi.fn(),
}));

vi.mock("@/hooks/use-panel-actions", () => ({
  usePanelActions: () => ({
    addBrowser: vi.fn(),
    addTerminal: vi.fn(),
    addPlan: vi.fn(),
    addChanges: vi.fn(),
  }),
}));

vi.mock("@/components/vcs/vcs-dialogs", () => ({
  useVcsDialogs: () => ({
    openCommitDialog: vi.fn(),
    openPRDialog: vi.fn(),
  }),
}));

vi.mock("@/hooks/use-task-archive-confirm", () => ({
  useTaskArchiveConfirm: () => ({
    target: null,
    requestArchive: vi.fn(),
    closeConfirm: vi.fn(),
    confirmArchive: vi.fn(),
    isPending: false,
  }),
}));

vi.mock("@/components/task/new-session-dialog", () => ({
  NewSessionDialog: () => null,
}));

vi.mock("@/components/task/new-subtask-dialog", () => ({
  NewSubtaskDialog: () => null,
}));

vi.mock("@/components/task/task-archive-confirmation", () => ({
  TaskArchiveConfirmation: () => null,
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mocks.underlyingCancel }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
  useAppStoreApi: () => ({
    getState: () => ({ ...mocks.state, setCancelTurnPending: mocks.setCancelTurnPending }),
  }),
}));

import { SessionCommands } from "@/components/session-commands";
import { QuickChatCancelCommands } from "./quick-chat-cancel-commands";

function CommandsProbe({ onCommands }: { onCommands: (commands: CommandItem[]) => void }) {
  const commands = useCommands();

  useEffect(() => onCommands(commands), [commands, onCommands]);

  return null;
}

function cancelCommand(commands: CommandItem[]) {
  return commands.find((command) => command.id === "session-cancel");
}

function cancellationCommands(commands: CommandItem[]) {
  return commands.filter((command) => command.id === "session-cancel");
}

async function expectCancellationCount(onCommands: ReturnType<typeof vi.fn>, count: number) {
  await waitFor(() =>
    expect(cancellationCommands(onCommands.mock.lastCall?.[0] ?? [])).toHaveLength(count),
  );
  return cancellationCommands(onCommands.mock.lastCall?.[0] ?? []);
}

// eslint-disable-next-line max-lines-per-function -- registry lifecycle cases share one focused harness.
describe("QuickChatCancelCommands", () => {
  beforeEach(() => {
    mocks.state.quickChat.isOpen = true;
    mocks.state.quickChat.activeKind = "conversation";
    mocks.state.quickChat.activeSessionId = "quick-session";
    mocks.state.taskSessions.items["quick-session"].cancellation_pending = false;
    mocks.state.taskSessions.items["quick-session-b"].cancellation_pending = false;
    mocks.state.taskSessions.items["task-session"].cancellation_pending = false;
    mocks.state.chatInput.cancellingBySessionId = {};
    mocks.underlyingCancel.mockClear();
  });

  afterEach(() => cleanup());

  it("registers cancellation only for the active structured conversation", async () => {
    const onCancel = vi.fn(async () => {});
    const onCommands = vi.fn();
    const { rerender } = render(
      <CommandRegistryProvider>
        <QuickChatCancelCommands sessionId="quick-session" isWorking onCancel={onCancel} />
        <CommandsProbe onCommands={onCommands} />
      </CommandRegistryProvider>,
    );

    await waitFor(() => expect(cancelCommand(onCommands.mock.lastCall?.[0] ?? [])).toBeDefined());
    await cancelCommand(onCommands.mock.lastCall?.[0] ?? [])?.action?.();
    expect(onCancel).toHaveBeenCalledOnce();

    mocks.state.quickChat.activeKind = "terminal";
    rerender(
      <CommandRegistryProvider>
        <QuickChatCancelCommands sessionId="quick-session" isWorking onCancel={onCancel} />
        <CommandsProbe onCommands={onCommands} />
      </CommandRegistryProvider>,
    );
    await waitFor(() => expect(cancelCommand(onCommands.mock.lastCall?.[0] ?? [])).toBeUndefined());
  });

  it("ignores a second cancellation while the first request is pending", async () => {
    let resolveCancel: (() => void) | undefined;
    const onCancel = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveCancel = resolve;
        }),
    );
    const onCommands = vi.fn();
    render(
      <CommandRegistryProvider>
        <QuickChatCancelCommands sessionId="quick-session" isWorking onCancel={onCancel} />
        <CommandsProbe onCommands={onCommands} />
      </CommandRegistryProvider>,
    );

    await waitFor(() => expect(cancelCommand(onCommands.mock.lastCall?.[0] ?? [])).toBeDefined());
    const command = cancelCommand(onCommands.mock.lastCall?.[0] ?? []);
    if (!command?.action) throw new Error("Missing Quick Chat cancellation command");

    const firstRequest = command.action();
    await waitFor(() => expect(onCancel).toHaveBeenCalledOnce());
    await command.action();
    expect(onCancel).toHaveBeenCalledOnce();

    resolveCancel?.();
    await firstRequest;
  });

  it("targets only the active Quick Chat session over a running task while switching and closing", async () => {
    const quickA = vi.fn(async () => {});
    const quickB = vi.fn(async () => {});
    const onCommands = vi.fn();
    const { rerender } = render(
      <CommandRegistryProvider>
        <SessionCommands sessionId="task-session" isAgentRunning />
        <QuickChatCancelCommands sessionId="quick-session" isWorking onCancel={quickA} />
        <CommandsProbe onCommands={onCommands} />
      </CommandRegistryProvider>,
    );

    let commands = await expectCancellationCount(onCommands, 1);
    await commands[0].action?.();
    expect(quickA).toHaveBeenCalledOnce();
    expect(mocks.underlyingCancel).not.toHaveBeenCalled();

    mocks.state.quickChat.activeSessionId = "quick-session-b";
    rerender(
      <CommandRegistryProvider>
        <SessionCommands sessionId="task-session" isAgentRunning />
        <QuickChatCancelCommands sessionId="quick-session-b" isWorking onCancel={quickB} />
        <CommandsProbe onCommands={onCommands} />
      </CommandRegistryProvider>,
    );
    commands = await expectCancellationCount(onCommands, 1);
    await commands[0].action?.();
    expect(quickB).toHaveBeenCalledOnce();
    expect(quickA).toHaveBeenCalledOnce();
    expect(mocks.underlyingCancel).not.toHaveBeenCalled();

    mocks.state.quickChat.isOpen = false;
    rerender(
      <CommandRegistryProvider>
        <SessionCommands sessionId="task-session" isAgentRunning />
        <CommandsProbe onCommands={onCommands} />
      </CommandRegistryProvider>,
    );
    commands = await expectCancellationCount(onCommands, 1);
    await commands[0].action?.();
    expect(mocks.underlyingCancel).toHaveBeenCalledWith(
      "agent.cancel",
      { session_id: "task-session" },
      15000,
    );

    mocks.state.quickChat.isOpen = true;
    rerender(
      <CommandRegistryProvider>
        <SessionCommands sessionId="task-session" isAgentRunning />
        <QuickChatCancelCommands sessionId="quick-session-b" isWorking onCancel={quickB} />
        <CommandsProbe onCommands={onCommands} />
      </CommandRegistryProvider>,
    );
    commands = await expectCancellationCount(onCommands, 1);
    await commands[0].action?.();
    expect(quickB).toHaveBeenCalledTimes(2);
    expect(mocks.underlyingCancel).toHaveBeenCalledOnce();
  });

  it.each([
    ["setup", "conversation", false, null],
    ["terminal", "terminal", true, null],
    ["idle", "conversation", false, null],
    [
      "disconnected clarification",
      "conversation",
      true,
      { metadata: { agent_disconnected: true } },
    ],
  ] as const)(
    "registers no cancellation command for %s Quick Chat state",
    async (_name, activeKind, isWorking, pendingClarification) => {
      const onCommands = vi.fn();
      const { rerender } = render(
        <CommandRegistryProvider>
          <SessionCommands sessionId="task-session" isAgentRunning />
          <QuickChatCancelCommands
            sessionId="quick-session"
            isWorking={isWorking}
            pendingClarification={pendingClarification as never}
            onCancel={vi.fn(async () => {})}
          />
          <CommandsProbe onCommands={onCommands} />
        </CommandRegistryProvider>,
      );

      mocks.state.quickChat.activeKind = activeKind;
      rerender(
        <CommandRegistryProvider>
          <SessionCommands sessionId="task-session" isAgentRunning />
          <QuickChatCancelCommands
            sessionId="quick-session"
            isWorking={isWorking}
            pendingClarification={pendingClarification as never}
            onCancel={vi.fn(async () => {})}
          />
          <CommandsProbe onCommands={onCommands} />
        </CommandRegistryProvider>,
      );
      await expectCancellationCount(onCommands, 0);
    },
  );

  it("rejects backend-pending cancellation and clears its failed request guard", async () => {
    const onCancel = vi
      .fn<() => Promise<void>>()
      .mockRejectedValueOnce(new Error("cancel failed"))
      .mockResolvedValueOnce(undefined);
    const onCommands = vi.fn();
    render(
      <CommandRegistryProvider>
        <QuickChatCancelCommands sessionId="quick-session" isWorking onCancel={onCancel} />
        <CommandsProbe onCommands={onCommands} />
      </CommandRegistryProvider>,
    );

    const commands = await expectCancellationCount(onCommands, 1);
    mocks.state.taskSessions.items["quick-session"].cancellation_pending = true;
    await commands[0].action?.();
    expect(onCancel).not.toHaveBeenCalled();

    mocks.state.taskSessions.items["quick-session"].cancellation_pending = false;
    await expect(commands[0].action?.()).rejects.toThrow("cancel failed");
    await commands[0].action?.();
    expect(onCancel).toHaveBeenCalledTimes(2);
  });
});

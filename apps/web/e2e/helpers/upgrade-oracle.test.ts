import { expect, it, vi } from "vitest";
import type { UpgradeBackend } from "../fixtures/upgrade-backend";
import type { UpgradeSeed } from "./upgrade-seed";
import { assertPassiveUpgradeReads, assertPreserved } from "./upgrade-oracle";

it("detects changed, missing, duplicated and extra historical records", () => {
  const old = [{ id: "a", content: "old text", created_at: "2026-10-01" }];
  expect(() => assertPreserved(old, structuredClone(old))).not.toThrow();
  for (const actual of [
    [],
    [{ ...old[0], content: "changed" }],
    [old[0], old[0]],
    [...old, { ...old[0], id: "b" }],
    [{ ...old[0], created_at: "2026-10-02" }],
  ]) {
    expect(() => assertPreserved(old, actual)).toThrow(/preservation/);
  }
});

function passiveReadFixture() {
  const sessions = {
    waiting: [{ id: "waiting-session", state: "WAITING_FOR_INPUT" }],
    interrupted: [{ id: "interrupted-session", state: "WAITING_FOR_INPUT" }],
  };
  const messages: { id: string; author_type: string; type: string; content: string }[] = [];
  const turns = [{ id: "original-turn" }];
  const status = vi.fn(async () => ({ is_agent_running: false }));
  const backend = {
    api: {
      getTask: async (id: string) => ({ id }),
      listTaskSessions: async (id: keyof typeof sessions) => ({
        sessions: structuredClone(sessions[id]),
      }),
      listSessionMessages: async () => ({ messages }),
      listSessionTurns: async () => ({ turns }),
      getTaskPlan: async () => ({}),
      getUserSettings: async () => ({ settings: {} }),
      listWorkflowSteps: async () => ({ steps: [] }),
      listWorkspaces: async () => ({ workspaces: [{}] }),
      getRepository: async () => ({}),
      listWorkflows: async () => ({ workflows: [{}] }),
      getAgentProfile: async () => ({}),
      getExecutorProfile: async () => ({}),
      wsRequest: status,
    },
  } as unknown as UpgradeBackend;
  const seed = {
    tasks: { waiting: { id: "waiting" }, interrupted: { id: "interrupted" } },
  } as unknown as UpgradeSeed;
  return { backend, seed, status, messages, turns, sessions };
}

it("allows the interrupted conversation's automatic runtime restoration during passive reads", async () => {
  const { backend, seed, status, sessions } = passiveReadFixture();
  sessions.interrupted[0].state = "STARTING";
  status.mockResolvedValueOnce({ is_agent_running: false });
  status.mockImplementationOnce(async () => {
    sessions.interrupted[0].state = "WAITING_FOR_INPUT";
    return { is_agent_running: true };
  });
  await expect(assertPassiveUpgradeReads(backend, seed)).resolves.toBeUndefined();
});

it("rejects an execution launched for an ordinary waiting session", async () => {
  const { backend, seed, status } = passiveReadFixture();
  status.mockResolvedValueOnce({ is_agent_running: true });
  await expect(assertPassiveUpgradeReads(backend, seed)).rejects.toThrow(/waiting-session/);
});

it.each(["turn", "message", "session"])(
  "rejects a new %s created while reading session status",
  async (kind) => {
    const { backend, seed, status, messages, turns, sessions } = passiveReadFixture();
    status.mockResolvedValueOnce({ is_agent_running: false });
    status.mockImplementationOnce(async () => {
      if (kind === "turn") turns.push({ id: "unexpected-tool-only-turn" });
      if (kind === "message")
        messages.push({
          id: "replayed",
          author_type: "user",
          type: "message",
          content: "old prompt",
        });
      if (kind === "session")
        sessions.waiting.push({ id: "unexpected-session", state: "WAITING_FOR_INPUT" });
      return { is_agent_running: true };
    });
    await expect(assertPassiveUpgradeReads(backend, seed)).rejects.toThrow(/preservation/);
  },
);

it("reports the field path rather than silently dropping changed relationships", () => {
  expect(() =>
    assertPreserved({ session: { task_id: "old" } }, { session: { task_id: "other" } }),
  ).toThrow(/session.task_id/);
});

it("preserves original message identities, contents and order while allowing appended turns", async () => {
  const { assertHistoricalMessagesPreserved } = await import("./upgrade-oracle");
  const messages = [
    { id: "user", content: "original request" },
    { id: "agent", content: "original response" },
  ];
  const snapshot = (history: unknown[]) => ({
    tasks: [{ id: "task", sessions: [{ id: "session", messages: history }] }],
  });
  expect(() =>
    assertHistoricalMessagesPreserved(
      snapshot(messages),
      snapshot([...messages, { id: "new", content: "follow-up" }]),
    ),
  ).not.toThrow();
  for (const history of [
    messages.slice(1),
    [{ ...messages[0], content: "changed" }, messages[1]],
    [messages[0], { ...messages[1], id: "copied" }],
    [...messages].reverse(),
    [...messages, messages[0]],
  ])
    expect(() => assertHistoricalMessagesPreserved(snapshot(messages), snapshot(history))).toThrow(
      /preservation/,
    );
  expect(() => assertHistoricalMessagesPreserved(snapshot(messages), { tasks: [] })).toThrow(
    /preservation/,
  );
});

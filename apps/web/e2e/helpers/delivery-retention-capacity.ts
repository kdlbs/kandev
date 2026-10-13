import { randomUUID } from "node:crypto";
import path from "node:path";
import { expect } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { ApiClient } from "./api-client";
import type { SessionPage } from "../pages/session-page";
import { readMockACPTrace } from "./transient-turn-runtime-continuity";
import { retentionEvidence } from "./delivery-retention-compatibility";

export async function createRetentionCapacityProfile(api: ApiClient, backend: BackendContext) {
  const agent = (await api.listAgents()).agents.find((row) => row.name === "mock-agent");
  if (!agent) throw new Error("mock agent is unavailable");
  const tracePath = path.join(backend.tmpDir, `retention-capacity-${randomUUID()}.jsonl`);
  const profile = await api.createAgentProfile(agent.id, "Delivery retention capacity", {
    model: "mock-fast",
    auto_fallback: false,
    env_vars: [{ key: "E2E_MOCK_AGENT_ACP_TRACE_FILE", value: tracePath }],
  });
  return { profileId: profile.id, tracePath };
}

export async function assertCapacityContinuationAfterPruning(
  api: ApiClient,
  backend: BackendContext,
  session: SessionPage,
  context: { taskId: string; sessionId: string; tracePath: string },
) {
  const { taskId, sessionId, tracePath } = context;
  // A restart restores the workspace before the agent. Complete a normal turn
  // before measuring retained-runtime continuation, rather than queueing at startup.
  await session.sendMessageViaButton("/slow 2s");
  await expect(
    session.chat.getByText("Slow response complete after 2s.", { exact: false }),
  ).toBeVisible({ timeout: 60_000 });
  await expect
    .poll(
      async () =>
        (await api.listTaskSessions(taskId)).sessions.find((row) => row.id === sessionId)?.state,
      { timeout: 30_000 },
    )
    .toBe("WAITING_FOR_INPUT");
  const traceStart = readMockACPTrace(tracePath).length;
  const previousSubmissions = retentionEvidence(backend, sessionId).prompts.map((row) => row.id);
  await session.sendMessageViaButton("/capacity-completed-tools");
  await expect(
    session.chat.getByText(
      "Mock provider continued the unfinished request without repeating completed work.",
      { exact: true },
    ),
  ).toBeVisible({ timeout: 90_000 });
  await expect
    .poll(
      async () =>
        (await api.listTaskSessions(taskId)).sessions.find((row) => row.id === sessionId)?.state,
      { timeout: 30_000 },
    )
    .toBe("WAITING_FOR_INPUT");
  const trace = readMockACPTrace(tracePath).slice(traceStart);
  const prompts = trace.filter((row) => row.event === "prompt");
  expect(prompts).toHaveLength(2);
  expect(prompts.filter((row) => row.prompt?.includes("/capacity-completed-tools"))).toHaveLength(
    1,
  );
  expect(trace.filter((row) => row.event === "completed_side_effect")).toHaveLength(1);
  for (const prompt of prompts) {
    expect(prompt.process_id).toBeTruthy();
    expect(prompt.connection_id).toBeTruthy();
    expect(prompt.session_id).toBeTruthy();
  }
  expect(new Set(prompts.map((row) => row.process_id)).size).toBe(1);
  expect(new Set(prompts.map((row) => row.connection_id)).size).toBe(1);
  expect(new Set(prompts.map((row) => row.session_id)).size).toBe(1);
  const newSubmissions = retentionEvidence(backend, sessionId).prompts.filter(
    (row) => !previousSubmissions.includes(row.id),
  );
  expect(newSubmissions).toHaveLength(2);
  expect(new Set(newSubmissions.map((row) => row.id)).size).toBe(2);
  expect(
    (await api.listSessionMessages(sessionId)).messages.filter(
      (row) => row.author_type === "user" && row.content.trim() === "/capacity-completed-tools",
    ),
  ).toHaveLength(1);
}

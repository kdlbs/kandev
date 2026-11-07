import { expect } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import {
  assertRetainedACPTrace,
  createRetainedCapacityFixture,
  readMockACPTrace,
} from "./transient-turn-runtime-continuity";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import { pollUntil } from "./poll-until";
import type { SessionPage } from "../pages/session-page";

export const GENERIC_PROMPT_ERROR = "Internal error: Failed to execute statement";

export type PromptErrorFixture = Awaited<ReturnType<typeof createPromptErrorFixture>>;

/**
 * Seeds an interactive mock-agent task whose first prompt fails with a plain
 * JSON-RPC error: no capacity marker and no transient keyword. The runtime must
 * survive purely because the peer returned the error.
 */
export async function createPromptErrorFixture(
  backend: BackendContext,
  apiClient: ApiClient,
  seedData: SeedData,
  command = "/prompt-error:1",
) {
  const fixture = await createRetainedCapacityFixture(
    backend,
    apiClient,
    seedData,
    "generic-error",
    {
      initialPrompt: command,
    },
  );
  return { ...fixture, taskTitle: "Retained capacity generic-error" };
}

export async function waitForPromptError(apiClient: ApiClient, sessionId: string) {
  return pollUntil(
    async () => {
      const { messages } = await apiClient.listSessionMessages(sessionId);
      return messages.find(
        (message) =>
          message.metadata?.runtime_retained === true &&
          message.metadata?.failure_scope === "turn" &&
          message.content?.includes(GENERIC_PROMPT_ERROR) === true,
      );
    },
    (message): message is NonNullable<typeof message> => message !== undefined,
    180_000,
    "waiting for the retained generic prompt error",
  );
}

export function promptEventCount(tracePath: string): number {
  return readMockACPTrace(tracePath).filter((record) => record.event === "prompt").length;
}

export function assertPromptErrorACPTrace(tracePath: string, expectedPrompts: number) {
  assertRetainedACPTrace(tracePath, expectedPrompts);
}

export async function expectPromptErrorRow(
  session: SessionPage,
  options: { dismiss?: boolean; continue?: boolean } = {},
) {
  const row = session.activeChat().getByTestId("session-recovery-action-message");
  await expect(row).toHaveCount(1);
  await expect(row).toContainText(GENERIC_PROMPT_ERROR);
  if (options.dismiss === false) {
    await expect(row.getByTestId("turn-error-dismiss-button")).toHaveCount(0);
  } else {
    await expect(row.getByTestId("turn-error-dismiss-button")).toBeVisible({ timeout: 30_000 });
  }
  if (options.continue === false) {
    await expect(row.getByTestId("turn-error-continue-button")).toHaveCount(0);
  } else if (options.continue === true) {
    await expect(row.getByTestId("turn-error-continue-button")).toBeVisible();
  }
  return row;
}

export async function waitForUserContinueMessages(apiClient: ApiClient, sessionId: string) {
  return pollUntil(
    async () => {
      const { messages } = await apiClient.listSessionMessages(sessionId);
      return messages.filter(
        (message) => message.author_type === "user" && message.content === "continue",
      );
    },
    (messages) => messages.length > 0,
    60_000,
    "waiting for the manual continue message",
  );
}

export async function waitForAgentResponseAfter(
  apiClient: ApiClient,
  sessionId: string,
  after: string,
) {
  const afterTimestamp = parseStrictRfc3339Timestamp(after);
  return pollUntil(
    async () => {
      const { messages } = await apiClient.listSessionMessages(sessionId);
      return messages.find((message) => {
        const createdAt = parseStrictRfc3339Timestamp(message.created_at);
        return (
          message.author_type === "agent" &&
          afterTimestamp !== null &&
          createdAt !== null &&
          createdAt > afterTimestamp &&
          (message.content ?? "").trim().length > 0
        );
      });
    },
    (message): message is NonNullable<typeof message> => message !== undefined,
    60_000,
    "waiting for the agent response after the manual continuation",
  );
}

export function originalPromptOccurrences(tracePath: string, command: string): number {
  return readMockACPTrace(tracePath).filter(
    (record) => record.event === "prompt" && record.prompt?.includes(command),
  ).length;
}

export async function expectTurnErrorBadge(session: SessionPage, taskTitle: string) {
  await expect(
    session.sidebarTaskItem(taskTitle).first().getByTestId("task-agent-error-icon"),
  ).toBeVisible({ timeout: 15_000 });
}

export async function expectNoTurnErrorBadge(session: SessionPage, taskTitle: string) {
  await expect(
    session.sidebarTaskItem(taskTitle).first().getByTestId("task-agent-error-icon"),
  ).toBeHidden({ timeout: 15_000 });
}

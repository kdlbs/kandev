import { expect } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import {
  assertRetainedACPTrace,
  createRetainedCapacityFixture,
  readMockACPTrace,
} from "./transient-turn-runtime-continuity";
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
  options: { dismiss?: boolean } = {},
) {
  const row = session.activeChat().getByTestId("session-recovery-action-message");
  await expect(row).toHaveCount(1);
  await expect(row).toContainText(GENERIC_PROMPT_ERROR);
  if (options.dismiss === false) {
    await expect(row.getByTestId("turn-error-dismiss-button")).toHaveCount(0);
  } else {
    await expect(row.getByTestId("turn-error-dismiss-button")).toBeVisible({ timeout: 30_000 });
  }
  return row;
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

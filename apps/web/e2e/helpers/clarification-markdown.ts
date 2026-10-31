import { expect, type Locator, type Page, type Route } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";

export const CLARIFICATION_REFERENCE_URL = "https://example.com/storage";
export const LONG_REFERENCE_LABEL = "retention".repeat(20);

export async function seedMarkdownHistory(apiClient: ApiClient, seedData: SeedData, title: string) {
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "WAITING_FOR_INPUT",
  });
  const pendingId = `markdown-${task.id}`;
  const question = {
    id: "markdown",
    prompt: `Read [storage guide](${CLARIFICATION_REFERENCE_URL}).\nCompare **retention** with \`${"configuration".repeat(20)}\`.\n\n\`\`\`sh\npnpm build\npnpm test\n\`\`\`\n\n1. Check retention\n2. Compare [${LONG_REFERENCE_LABEL}](${CLARIFICATION_REFERENCE_URL})`,
    options: [{ option_id: "postgres", label: "`Postgres`", description: "" }],
  };
  const startedAt = new Date(Date.now() - 120_000).toISOString();
  await apiClient.seedSessionMessage(sessionId, {
    type: "clarification_request",
    newTurn: true,
    turnStartedAt: startedAt,
    turnCompletedAt: startedAt,
    metadata: {
      pending_id: pendingId,
      session_id: sessionId,
      question_id: question.id,
      question_index: 0,
      question_total: 1,
      status: "pending",
      context: "Keep `context` literal.",
      question,
    },
  });
  await apiClient.seedSessionMessage(sessionId, {
    type: "message",
    content: "Starting a different approach.",
    newTurn: true,
    turnStartedAt: new Date(Date.now() - 60_000).toISOString(),
  });
  return { taskId: task.id, sessionId, pendingId, question };
}

export async function expectClarificationReferencePopup(
  page: Page,
  activate: () => Promise<unknown>,
  url = CLARIFICATION_REFERENCE_URL,
) {
  const context = page.context();
  const fulfill = (route: Route) =>
    route.fulfill({ contentType: "text/html", body: "<title>Storage guide</title>" });
  await context.route(url, fulfill);
  try {
    const [popup] = await Promise.all([page.waitForEvent("popup"), activate()]);
    try {
      await expect(popup).toHaveURL(url);
      await expect(popup).toHaveTitle("Storage guide");
    } finally {
      await popup.close();
    }
  } finally {
    await context.unroute(url, fulfill);
  }
}

export async function expectTruncatedQuestionSummary(summary: Locator) {
  await expect(summary).toHaveCount(1);
  const styles = await summary.evaluate((element) => {
    const style = getComputedStyle(element);
    return {
      whiteSpace: style.whiteSpace,
      overflow: style.overflow,
      textOverflow: style.textOverflow,
      height: element.getBoundingClientRect().height,
      lineHeight: parseFloat(style.lineHeight),
      descendantWhiteSpace: Array.from(
        element.querySelectorAll("*"),
        (node) => getComputedStyle(node).whiteSpace,
      ),
    };
  });
  expect(styles.whiteSpace).toBe("nowrap");
  expect(styles.overflow).toBe("hidden");
  expect(styles.textOverflow).toBe("ellipsis");
  expect(styles.height).toBeLessThanOrEqual(styles.lineHeight + 1);
  expect(new Set(styles.descendantWhiteSpace)).toEqual(new Set(["nowrap"]));
}

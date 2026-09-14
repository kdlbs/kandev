import { expect, type Locator } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

export const LARGE_COLUMN_TASK_COUNT = 440;
const MAX_MOUNTED_TASK_CARDS = 50;
const TASK_SEED_BATCH_SIZE = 25;

export async function seedLargeColumnTasks(
  apiClient: ApiClient,
  seedData: SeedData,
  titlePrefix: string,
  count = LARGE_COLUMN_TASK_COUNT,
): Promise<void> {
  for (let start = 0; start < count; start += TASK_SEED_BATCH_SIZE) {
    const batchCount = Math.min(TASK_SEED_BATCH_SIZE, count - start);
    await Promise.all(
      Array.from({ length: batchCount }, (_, offset) =>
        apiClient.createTask(seedData.workspaceId, `${titlePrefix} ${start + offset + 1}`, {
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
        }),
      ),
    );
  }
}

export function taskCards(column: Locator): Locator {
  return column.locator('[data-slot="card"][data-testid^="task-card-"]');
}

export async function mountedTaskCardIds(column: Locator): Promise<string[]> {
  return taskCards(column).evaluateAll((cards) =>
    cards
      .map((card) => card.getAttribute("data-testid"))
      .filter((testId): testId is string => Boolean(testId)),
  );
}

export async function expectBoundedMountedCards(column: Locator): Promise<void> {
  await expect.poll(() => taskCards(column).count()).toBeLessThan(MAX_MOUNTED_TASK_CARDS);
}

export async function scrollColumnToBottom(scrollOwner: Locator): Promise<void> {
  let stableBottomSamples = 0;
  await expect
    .poll(
      async () => {
        const atBottom = await scrollOwner.evaluate((element) => {
          const maxScrollTop = Math.max(0, element.scrollHeight - element.clientHeight);
          if (maxScrollTop === 0) return false;
          element.scrollTop = maxScrollTop;
          element.dispatchEvent(new Event("scroll", { bubbles: true }));
          return element.scrollTop >= maxScrollTop - 1;
        });
        stableBottomSamples = atBottom ? stableBottomSamples + 1 : 0;
        return stableBottomSamples >= 2;
      },
      {
        timeout: 10_000,
        intervals: [50, 100, 250],
        message: "virtualized column did not settle at the bottom",
      },
    )
    .toBe(true);
}

import { expect, type Locator, type Page, type TestInfo } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { createStandardProfile, openTaskSession } from "../../helpers/git-helper";
import { waitForLatestSessionDone } from "../../helpers/session";
import type { ThreadViewApi, ThreadViewDraftApi } from "../../../lib/types/http-user-settings";

export type ThreadPresentationSettings = {
  thread_views: ThreadViewApi[];
  thread_active_view_id: string;
  thread_view_draft: ThreadViewDraftApi | null;
};

export async function captureThreadSettings(
  apiClient: ApiClient,
): Promise<ThreadPresentationSettings> {
  const { settings } = await apiClient.getUserSettings();
  return {
    thread_views: settings.thread_views as ThreadViewApi[],
    thread_active_view_id: settings.thread_active_view_id as string,
    thread_view_draft: settings.thread_view_draft as ThreadViewDraftApi | null,
  };
}

export async function seedThreadPresentation(
  apiClient: ApiClient,
  presentation: {
    layout: "columns" | "grid";
    autoHideComposer?: boolean;
    maxChats?: number | null;
  },
) {
  await apiClient.saveUserSettings({
    thread_views: [
      {
        id: "view-presentation",
        name: "Presentation",
        task_scope: { mode: "all", task_ids: [] },
        filters: [],
        sort: { key: "title", direction: "asc" },
        max_columns: presentation.maxChats === undefined ? 5 : presentation.maxChats,
        layout: presentation.layout,
        auto_hide_composer: presentation.autoHideComposer ?? false,
      },
    ],
    thread_active_view_id: "view-presentation",
    thread_view_draft: null,
  });
}

export async function startPresentationThread(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  description = "/e2e:simple-message",
) {
  const profile = await createStandardProfile(apiClient, `presentation-${title}`);
  const task = await apiClient.createTaskWithAgent(seedData.workspaceId, title, profile.id, {
    description,
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  await openTaskSession(page, title);
  await waitForLatestSessionDone(apiClient, task.id, 1, `presentation turn for ${title}`);
  return task;
}

export async function threadBoxes(board: Locator) {
  return board.locator("[data-thread-column-id]").evaluateAll((tiles) =>
    tiles.map((tile) => {
      const box = tile.getBoundingClientRect();
      return {
        id: tile.getAttribute("data-thread-column-id"),
        x: box.x,
        y: box.y,
        width: box.width,
        height: box.height,
      };
    }),
  );
}

export async function expectTwoRows(board: Locator) {
  await expect
    .poll(async () => {
      const [first, second, third] = await threadBoxes(board);
      return (
        first &&
        second &&
        third &&
        Math.abs(first.x - second.x) < 1 &&
        second.y > first.y + first.height &&
        third.x > first.x &&
        Math.abs(first.y - third.y) < 1
      );
    })
    .toBe(true);
}

export async function capturePresentation(page: Page, testInfo: TestInfo, name: string) {
  const path = testInfo.outputPath(`${name}.png`);
  await page.screenshot({ path, animations: "disabled" });
  await testInfo.attach(name, { path, contentType: "image/png" });
}

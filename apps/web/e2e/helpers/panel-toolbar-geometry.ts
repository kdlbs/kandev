import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";

export async function expectFileBrowserIconCentered(
  button: Locator,
  iconIndex: 0 | 1,
  state: string,
) {
  const icons = button.locator("svg");
  await expect(icons).toHaveCount(2);
  const icon = icons.nth(iconIndex);
  await expect(icon).toBeVisible();
  await expect
    .poll(() => icon.evaluate((element) => Number(getComputedStyle(element).opacity)), {
      message: `Waiting for the ${state} Files copy-path icon to become visible`,
    })
    .toBeGreaterThan(0.99);

  const [buttonBox, iconBox] = await Promise.all([button.boundingBox(), icon.boundingBox()]);
  expect(buttonBox, `${state} Files copy-path target`).not.toBeNull();
  expect(iconBox, `${state} Files copy-path icon`).not.toBeNull();
  expect(
    Math.abs(buttonBox!.x + buttonBox!.width / 2 - (iconBox!.x + iconBox!.width / 2)),
    `${state} Files copy-path icon horizontal center offset`,
  ).toBeLessThanOrEqual(1);
  expect(
    Math.abs(buttonBox!.y + buttonBox!.height / 2 - (iconBox!.y + iconBox!.height / 2)),
    `${state} Files copy-path icon vertical center offset`,
  ).toBeLessThanOrEqual(1);
}

export async function readTaskWorkspacePath(page: Page, apiClient: ApiClient): Promise<string> {
  const taskId = new URL(page.url()).pathname.match(/^\/t\/([^/]+)/)?.[1];
  if (!taskId) throw new Error("Expected the task detail route to include a task ID");

  let workspacePath = "";
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        workspacePath = sessions[0]?.workspace_path ?? sessions[0]?.worktree_path ?? "";
        return workspacePath;
      },
      { timeout: 20_000, message: "Waiting for the task workspace path" },
    )
    .not.toBe("");
  return workspacePath;
}

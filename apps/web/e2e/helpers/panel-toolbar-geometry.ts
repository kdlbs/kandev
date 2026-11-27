import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";

export async function expectFileBrowserIconCentered(
  button: Locator,
  iconIndex: 0 | 1,
  state: string,
) {
  await expect(async () => {
    // Copy feedback replaces the SVG. Read visibility and geometry together.
    const geometry = await button.evaluate((element, index) => {
      const icons = element.querySelectorAll("svg");
      const icon = icons[index];
      if (!icon) return null;
      const target = element.getBoundingClientRect();
      const rect = icon.getBoundingClientRect();
      const style = getComputedStyle(icon);
      return {
        iconCount: icons.length,
        targetVisible: target.width > 0 && target.height > 0,
        iconVisible: rect.width > 0 && rect.height > 0 && style.visibility === "visible",
        opacity: Number(style.opacity),
        copied: icon.classList.contains("tabler-icon-check"),
        offsetX: Math.abs(target.x + target.width / 2 - (rect.x + rect.width / 2)),
        offsetY: Math.abs(target.y + target.height / 2 - (rect.y + rect.height / 2)),
      };
    }, iconIndex);
    expect(geometry, `${state} Files copy-path geometry`).not.toBeNull();
    expect(geometry!.iconCount).toBe(2);
    expect(geometry!.targetVisible, `${state} Files copy-path target`).toBe(true);
    expect(geometry!.iconVisible, `${state} Files copy-path icon`).toBe(true);
    expect(geometry!.opacity).toBeGreaterThan(0.99);
    if (state.startsWith("copied")) expect(geometry!.copied).toBe(true);
    expect(
      geometry!.offsetX,
      `${state} Files copy-path icon horizontal center offset`,
    ).toBeLessThanOrEqual(1);
    expect(
      geometry!.offsetY,
      `${state} Files copy-path icon vertical center offset`,
    ).toBeLessThanOrEqual(1);
  }).toPass({ timeout: 5_000 });
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

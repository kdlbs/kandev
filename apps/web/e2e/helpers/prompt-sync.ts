import type { Locator, Page } from "@playwright/test";
import { expect } from "../fixtures/test-base";
import { waitForHttp } from "./causal-waits";
import type { ApiClient } from "./api-client";

export const GITLAB_PROJECT = "platform/prompts";
export const PROMPTS_DIR = ".kandev/prompts";
export const SYNC_PATH = /^\/api\/v1\/prompt-sync\/sync$/;

export function promptRow(page: Page, name: string) {
  return page.locator(`[data-testid="prompt-list-item"][data-prompt-name="${name}"]`);
}

// seedPromptFiles replaces the mock repository's files. The first call also
// connects GitLab: the testPage fixture resets the workspace when a test
// starts, so the connection is created inside the test body.
export async function seedPromptFiles(
  apiClient: ApiClient,
  workspaceId: string,
  files: Record<string, string>,
  { first = false }: { first?: boolean } = {},
) {
  if (first) await apiClient.configureGitLab(workspaceId);
  else await apiClient.mockGitLabReset(workspaceId);
  await apiClient.mockGitLabAddRepoFiles(
    workspaceId,
    GITLAB_PROJECT,
    "main",
    Object.entries(files).map(([name, content]) => ({ path: `${PROMPTS_DIR}/${name}`, content })),
  );
}

export async function syncNow(page: Page) {
  const response = waitForHttp(page, "POST", SYNC_PATH);
  await page.getByTestId("prompt-sync-now").click();
  expect((await response).ok()).toBe(true);
}

// configureThroughDialog sets up a GitLab source. With tap, every activation
// uses touch input so the phone path is exercised end to end.
export async function configureThroughDialog(page: Page, tap: boolean) {
  const activate = (target: Locator) => (tap ? target.tap() : target.click());
  await activate(page.getByTestId("prompt-sync-open"));
  const dialog = page.getByTestId("prompt-sync-dialog");
  await expect(dialog).toBeVisible();
  await activate(dialog.getByRole("tab", { name: "GitLab" }));
  await dialog.getByTestId("prompt-sync-url-input").fill(GITLAB_PROJECT);
  await activate(dialog.getByTestId("prompt-sync-poll-toggle"));
  const saved = waitForHttp(page, "POST", /^\/api\/v1\/prompt-sync\/config$/);
  await activate(dialog.getByTestId("prompt-sync-save"));
  expect((await saved).ok()).toBe(true);
  await expect(dialog).toHaveCount(0);
}

// cleanUpPromptSync removes the install-wide configuration and every
// user-created prompt, so the next spec starts without synced prompts.
export async function cleanUpPromptSync(apiClient: ApiClient) {
  await apiClient.rawRequest("DELETE", "/api/v1/prompt-sync/config");
  const { prompts } = await apiClient.listPrompts();
  for (const prompt of prompts) {
    if (!prompt.builtin) await apiClient.deletePrompt(prompt.id).catch(() => undefined);
  }
}

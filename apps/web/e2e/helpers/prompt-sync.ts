import type { Locator, Page, Response } from "@playwright/test";
import { expect } from "../fixtures/test-base";
import { waitForHttp } from "./causal-waits";
import type { ApiClient } from "./api-client";

export const GITLAB_PROJECT = "platform/prompts";
export const PROMPTS_DIR = ".kandev/prompts";
export const SYNC_PATH = /^\/api\/v1\/prompt-sync\/sync$/;
export const CONFIG_PATH = /^\/api\/v1\/prompt-sync\/config$/;
export const CHECK_PATH = /^\/api\/v1\/prompt-sync\/check$/;
export const VERSIONS_PATH = /^\/api\/v1\/prompt-sync\/prompts\/[^/]+\/versions$/;

/**
 * Saves pending version switches from the page's save bar and waits for one
 * versions write per switched prompt.
 */
export async function saveVersions(page: Page, { tap = false, count = 1 } = {}) {
  const statuses: number[] = [];
  const written = new Promise<void>((resolve) => {
    const onResponse = (response: Response) => {
      const request = response.request();
      if (request.method() !== "PUT" || !VERSIONS_PATH.test(new URL(response.url()).pathname)) {
        return;
      }
      statuses.push(response.status());
      if (statuses.length < count) return;
      page.off("response", onResponse);
      resolve();
    };
    page.on("response", onResponse);
  });
  const button = page.getByTestId("settings-floating-save").getByRole("button", {
    name: "Save changes",
  });
  await (tap ? button.tap() : button.click());
  await written;
  expect(statuses.every((status) => status < 300)).toBe(true);
}

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
  { first = false, path = PROMPTS_DIR }: { first?: boolean; path?: string } = {},
) {
  if (first) await apiClient.configureGitLab(workspaceId);
  else await apiClient.mockGitLabReset(workspaceId);
  if (Object.keys(files).length === 0) return;
  await apiClient.mockGitLabAddRepoFiles(
    workspaceId,
    GITLAB_PROJECT,
    "main",
    Object.entries(files).map(([name, content]) => ({ path: `${path}/${name}`, content })),
  );
}

export async function syncNow(page: Page) {
  const response = waitForHttp(page, "POST", SYNC_PATH);
  await page.getByTestId("prompt-sync-now").click();
  expect((await response).ok()).toBe(true);
}

/** Makes the next Sync now response a quota refusal, then lets later requests through. */
export async function blockNextPromptSyncWithQuota(page: Page, apiClient: ApiClient) {
  const response = await apiClient.rawRequest("GET", "/api/v1/prompt-sync/config");
  expect(response.ok).toBe(true);
  const config = await response.json();
  let blocked = false;
  await page.route("**/api/v1/prompt-sync/sync", async (route) => {
    if (blocked) {
      await route.continue();
      return;
    }
    blocked = true;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        config: {
          ...config,
          last_synced_at: new Date().toISOString(),
          last_ok: false,
          last_error: "GitHub quota is temporarily unavailable",
          last_error_class: "github_rate_limited",
          next_attempt_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
        },
        error: "GitHub quota is temporarily unavailable",
        error_code: "github_rate_limited",
      }),
    });
  });
}

// fillSyncDialog opens the Prompt Sync dialog and enters a GitLab source.
// With tap, every activation uses touch input so the phone path is exercised
// end to end.
export async function fillSyncDialog(page: Page, tap: boolean) {
  const activate = (target: Locator) => (tap ? target.tap() : target.click());
  await activate(page.getByTestId("prompt-sync-open"));
  const dialog = page.getByTestId("prompt-sync-dialog");
  await expect(dialog).toBeVisible();
  await activate(dialog.getByRole("tab", { name: "GitLab" }));
  await dialog.getByTestId("prompt-sync-url-input").fill(GITLAB_PROJECT);
  await activate(dialog.getByTestId("prompt-sync-poll-toggle"));
  return dialog;
}

// configureThroughDialog sets up a GitLab source with no name conflicts:
// Save checks the source, then saves and runs the first sync.
export async function configureThroughDialog(page: Page, tap: boolean) {
  const dialog = await fillSyncDialog(page, tap);
  const saved = waitForHttp(page, "POST", CONFIG_PATH);
  await (tap
    ? dialog.getByTestId("prompt-sync-save").tap()
    : dialog.getByTestId("prompt-sync-save").click());
  expect((await saved).ok()).toBe(true);
  await expect(dialog).toHaveCount(0);
}

// saveToReview clicks Save on a filled dialog and waits for the check that
// opens the conflict review step instead of saving.
export async function saveToReview(page: Page, dialog: Locator, tap: boolean) {
  const checked = waitForHttp(page, "POST", CHECK_PATH);
  const save = dialog.getByTestId("prompt-sync-save");
  await (tap ? save.tap() : save.click());
  expect((await checked).ok()).toBe(true);
  await expect(dialog.getByTestId("prompt-conflict-review")).toBeVisible();
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

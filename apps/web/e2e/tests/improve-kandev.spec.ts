import type { Page } from "@playwright/test";
import { test, expect } from "../fixtures/test-base";

/**
 * E2E tests for the Improve Kandev dialog. The bootstrap endpoint is mocked
 * because it would otherwise clone https://github.com/kdlbs/kandev and shell
 * out to the gh CLI. The system/health endpoint is mocked to keep the intro
 * screen out of the GhAuthMissing branch regardless of backend state.
 */

const BOOTSTRAP_URL = "**/api/v1/system/improve-kandev/bootstrap";
const FRONTEND_LOG_URL = "**/api/v1/system/improve-kandev/bundle/frontend-log";
const HEALTH_URL = "**/api/v1/system/health";

type ForkStatus = "writable" | "ready" | "blocked_emu" | "unknown";

type BootstrapOverrides = {
  github_login?: string;
  has_write_access?: boolean;
  fork_status?: ForkStatus;
  fork_message?: string;
};

async function mockImproveKandevApis(
  page: Page,
  seed: { repositoryId: string; workflowId: string },
  overrides: BootstrapOverrides = {},
): Promise<void> {
  const bundleDir = "/tmp/kandev-improve-e2e";
  const hasWrite = overrides.has_write_access ?? false;
  // Default fork_status mirrors how the backend would respond for the given
  // write-access value: writable when the user has push access, otherwise
  // unknown (the safe fall-through that lets the dialog proceed normally).
  const forkStatus: ForkStatus = overrides.fork_status ?? (hasWrite ? "writable" : "unknown");

  await page.route(HEALTH_URL, (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ healthy: true, issues: [] }),
    }),
  );

  await page.route(BOOTSTRAP_URL, (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        repository_id: seed.repositoryId,
        workflow_id: seed.workflowId,
        branch: "main",
        bundle_dir: bundleDir,
        bundle_files: {
          metadata: `${bundleDir}/metadata.json`,
          backend_log: `${bundleDir}/backend.log`,
          frontend_log: `${bundleDir}/frontend.log`,
        },
        github_login: overrides.github_login ?? "octocat",
        has_write_access: hasWrite,
        fork_status: forkStatus,
        ...(overrides.fork_message ? { fork_message: overrides.fork_message } : {}),
      }),
    }),
  );

  await page.route(FRONTEND_LOG_URL, (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ path: `${bundleDir}/frontend.log` }),
    }),
  );
}

test.describe("Improve Kandev dialog", () => {
  test("intro → create flow shows workflow preview, useful info, and fork banner", async ({
    testPage,
    seedData,
  }) => {
    await mockImproveKandevApis(testPage, seedData, {
      github_login: "octocat",
      has_write_access: false,
    });

    await testPage.goto("/");

    await testPage.getByTestId("improve-kandev-button").first().click();

    // Intro screen
    const introDialog = testPage.getByRole("dialog", { name: "Improve Kandev" });
    await expect(introDialog).toBeVisible();
    await expect(introDialog.getByText(/KanDev is open source/)).toBeVisible();
    await expect(introDialog.getByText(/forks .* to your GitHub account/)).toBeVisible();

    const contribute = testPage.getByTestId("improve-kandev-proceed");
    await expect(contribute).toBeEnabled({ timeout: 10_000 });
    await contribute.click();

    // Create dialog mounts after Contribute is clicked
    const createDialog = testPage.getByTestId("create-task-dialog");
    await expect(createDialog).toBeVisible({ timeout: 10_000 });

    // Bug fix / Feature request kind tabs
    await expect(createDialog.getByRole("tab", { name: "Bug fix" })).toBeVisible();
    await expect(createDialog.getByRole("tab", { name: "Feature request" })).toBeVisible();

    // Contributor banner (fork mode)
    await expect(createDialog.getByText("@octocat")).toBeVisible();
    await expect(
      createDialog.getByText(/agent will fork kdlbs\/kandev to your account/i),
    ).toBeVisible();

    // Workflow preview header
    await expect(createDialog.getByText("Workflow", { exact: true })).toBeVisible();

    // Useful info collapsible expands and lists shell + skill commands
    const usefulInfoTrigger = createDialog.getByRole("button", { name: /useful commands/i });
    await expect(usefulInfoTrigger).toBeVisible();
    await usefulInfoTrigger.click();
    await expect(createDialog.getByText("make install && make dev")).toBeVisible();
    await expect(createDialog.getByText("/commit", { exact: true })).toBeVisible();
    await expect(createDialog.getByText("/pr-fixup", { exact: true })).toBeVisible();

    // GitHub URL toggle is hidden because the repository is locked to kandev
    await expect(createDialog.getByTestId("toggle-github-url")).toHaveCount(0);
  });

  test("contributor banner shows direct-push copy when user has write access", async ({
    testPage,
    seedData,
  }) => {
    await mockImproveKandevApis(testPage, seedData, {
      github_login: "kandev-maint",
      has_write_access: true,
    });

    await testPage.goto("/");
    await testPage.getByTestId("improve-kandev-button").first().click();

    const contribute = testPage.getByTestId("improve-kandev-proceed");
    await expect(contribute).toBeEnabled({ timeout: 10_000 });
    await contribute.click();

    const createDialog = testPage.getByTestId("create-task-dialog");
    await expect(createDialog).toBeVisible({ timeout: 10_000 });
    await expect(createDialog.getByText("@kandev-maint")).toBeVisible();
    await expect(
      createDialog.getByText(/push directly to a branch on the upstream repo/i),
    ).toBeVisible();
  });

  test("blocks contribution when account is detected as Enterprise Managed User", async ({
    testPage,
    seedData,
  }) => {
    const blockedMessage =
      "Your GitHub account appears to be an Enterprise Managed User (EMU) account, " +
      "which typically cannot fork repositories outside your owning enterprise. " +
      "The PR step would fail when forking kdlbs/kandev. Contact your GitHub admin " +
      "if you'd like to enable this, or contribute via another account.";
    await mockImproveKandevApis(testPage, seedData, {
      github_login: "alice_corp",
      has_write_access: false,
      fork_status: "blocked_emu",
      fork_message: blockedMessage,
    });

    await testPage.goto("/");
    await testPage.getByTestId("improve-kandev-button").first().click();

    const contribute = testPage.getByTestId("improve-kandev-proceed");
    await expect(contribute).toBeEnabled({ timeout: 10_000 });
    await contribute.click();

    // Blocked dialog mounts in place of the task-create form. The message
    // also fires as a toast, so scope the assertion to the dialog body.
    const blockedDialog = testPage.getByRole("dialog", { name: "Improve Kandev" });
    await expect(blockedDialog).toBeVisible({ timeout: 10_000 });
    await expect(blockedDialog.getByText(blockedMessage)).toBeVisible();

    // The task-create form must NOT mount: there is nothing to submit and
    // no fork should be attempted.
    await expect(testPage.getByTestId("create-task-dialog")).toHaveCount(0);

    // Close button is the only action; clicking it dismisses the dialog.
    await testPage.getByTestId("improve-kandev-blocked-close").click();
    await expect(blockedDialog).toBeHidden();
  });
});

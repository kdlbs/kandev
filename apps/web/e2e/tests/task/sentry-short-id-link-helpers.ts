import type { Locator, Page, Response, TestInfo } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { SessionPage } from "../../pages/session-page";

async function activate(control: Locator, mobile: boolean) {
  if (mobile) await control.tap();
  else await control.click();
}

export async function linkSentryIssue(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  {
    info,
    capture,
    mobile,
    issueKey,
    input,
    captureAsset,
    relink,
  }: {
    info: TestInfo;
    capture: PrAssetCapture;
    mobile: boolean;
    issueKey: string;
    input: string;
    captureAsset: boolean;
    relink?: boolean;
  },
) {
  const workspace = await api.createWorkspace(`Sentry link ${info.testId}`);
  try {
    const workflow = await api.createWorkflow(workspace.id, "Sentry link", "simple");
    const { steps } = await api.listWorkflowSteps(workflow.id);
    expect(steps.length).toBeGreaterThan(0);
    const sentry = await api.createSentryInstance({
      workspaceId: workspace.id,
      name: "Fixture Sentry",
      secret: "fixture-only-token",
    });
    await api.mockSentrySetAuthHealth({ instanceId: sentry.id, ok: true });
    await api.waitForIntegrationAuthHealthy("sentry", { workspaceId: workspace.id });
    const response = await api.rawRequest(
      "POST",
      `/api/v1/sentry/mock/issues?instanceId=${encodeURIComponent(sentry.id)}`,
      {
        issues: (relink ? [issueKey, "WEB-1B"] : [issueKey]).map((shortId, index) => ({
          id: String(123456 + index),
          shortId,
          title: "Fixture issue",
          permalink: `https://sentry.io/organizations/acme/issues/${123456 + index}/`,
          projectSlug: "web",
          level: "error",
          status: "unresolved",
        })),
      },
    );
    expect(response.ok).toBe(true);
    const repository = await api.createRepository(workspace.id, seed.repositoryPath);
    const task = await api.createTask(workspace.id, "Sentry fixture task", {
      workflow_id: workflow.id,
      workflow_step_id: steps[0].id,
      repository_ids: [repository.id],
      agent_profile_id: seed.agentProfileId,
      executor_profile_id: seed.worktreeExecutorProfileId,
    });
    await page.goto(`/t/${task.id}`);
    await new SessionPage(page).waitForLoad();
    let surface: Locator;
    if (mobile) {
      await page.getByTestId("mobile-task-picker-trigger").tap();
      surface = page.getByRole("dialog", { name: "Tasks", exact: true });
    } else {
      surface = page.getByTestId("task-sidebar");
    }
    const row = surface.locator(`[data-task-row-id="${task.id}"]`);
    const cases = relink
      ? [
          { issueKey, input },
          { issueKey: "WEB-1B", input: "WEB-1B" },
        ]
      : [{ issueKey, input }];
    for (const [index, current] of cases.entries()) {
      await submitSentryLink(page, api, row, surface, {
        ...current,
        taskId: task.id,
        workspaceId: workspace.id,
        instanceId: sentry.id,
        mobile,
      });
      if (captureAsset && index === cases.length - 1) await capture.screenshot("linked-sentry");
    }
    await info.attach(`sentry-link-${issueKey}`, {
      body: await page.screenshot({ path: info.outputPath(`sentry-link-${issueKey}.png`) }),
      contentType: "image/png",
    });
  } finally {
    await page.goto("/");
    await api.deleteWorkspace(workspace.id, workspace.name);
  }
}

async function submitSentryLink(
  page: Page,
  api: ApiClient,
  row: Locator,
  surface: Locator,
  {
    issueKey,
    input,
    taskId,
    workspaceId,
    instanceId,
    mobile,
  }: {
    issueKey: string;
    input: string;
    taskId: string;
    workspaceId: string;
    instanceId: string;
    mobile: boolean;
  },
) {
  await expect(row).toHaveCount(1);
  if (!mobile) await row.hover();
  await activate(row.getByRole("button", { name: "Task actions", exact: true }), mobile);
  const link = page.getByRole("menuitem", { name: "Link", exact: true });
  if (mobile) await link.tap();
  else await link.hover();
  await activate(page.getByRole("menuitem", { name: "Sentry Issue", exact: true }), mobile);
  const dialog = page.getByRole("dialog", { name: "Link Sentry issue", exact: true });
  await expect(dialog).toBeVisible();
  await dialog.getByTestId("task-external-link-input").fill(input);
  const replies: Response[] = [];
  const observe = (reply: Response) => {
    const url = new URL(reply.url());
    if (
      reply.request().method() === "GET" &&
      url.pathname === `/api/v1/sentry/issues/${issueKey}` &&
      url.searchParams.get("workspace_id") === workspaceId &&
      url.searchParams.get("instanceId") === instanceId
    )
      replies.push(reply);
  };
  page.on("response", observe);
  try {
    await activate(dialog.getByTestId("task-external-link-submit"), mobile);
    await expect(dialog).toBeHidden();
    await expect.poll(() => replies.length).toBe(1);
    expect(replies[0].status()).toBe(200);
    expect((await replies[0].json()).shortId).toBe(issueKey);
    await expect
      .poll(async () => (await api.getTask(taskId)).title)
      .toBe(`${issueKey}: Sentry fixture task`);
    if (mobile) {
      await page.getByTestId("mobile-task-picker-trigger").tap();
      await waitForFiniteAnimations(surface);
    }
    await expect(row).toBeVisible();
    await expect(row).toContainText(`${issueKey}: Sentry fixture task`);
  } finally {
    page.off("response", observe);
  }
}

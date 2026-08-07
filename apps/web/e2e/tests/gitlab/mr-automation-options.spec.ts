import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { seedGitLabReview, GITLAB_HOST, GITLAB_PROJECT } from "../../helpers/gitlab";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

const MR_IID = 210;

async function seedTaskWithLinkedMR(apiClient: ApiClient, seedData: SeedData, title: string) {
  await seedGitLabReview(apiClient, seedData.workspaceId, MR_IID, "MR automation options MR");
  await apiClient.updateRepository(seedData.repositoryId, {
    provider: "gitlab",
    provider_host: GITLAB_HOST,
    provider_owner: "platform",
    provider_name: "kandev",
  });
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await apiClient.linkTaskGitLabMR(seedData.workspaceId, {
    task_id: task.id,
    repository_id: seedData.repositoryId,
    mr_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${MR_IID}`,
  });
  return task.id;
}

async function openTask(testPage: import("@playwright/test").Page, taskId: string) {
  await testPage.goto(`/t/${taskId}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(testPage.getByTestId("mr-topbar-button")).toBeVisible({ timeout: 15_000 });
  return session;
}

// Automation controls live in the hover popover for a single linked MR on
// desktop (clicking the topbar button opens the MR detail panel directly —
// see mr-topbar-popover.spec.ts), so reaching them here means hovering.
async function openAutomationControls(testPage: import("@playwright/test").Page) {
  await testPage.getByTestId("mr-topbar-button").hover();
  const content = testPage.getByTestId("mr-automation-controls");
  await expect(content).toBeVisible();
  return content;
}

async function interceptLoadFailure(testPage: import("@playwright/test").Page) {
  await testPage.route("**/api/v1/gitlab/tasks/*/mr-automation", async (route) => {
    if (route.request().method() !== "GET") {
      await route.continue();
      return;
    }
    await route.fulfill({ status: 500, json: { error: "backend unavailable" } });
  });
}

test.describe("GitLab MR automation section (auto-fix / auto-merge)", () => {
  test("renders above Review follow-up and persists auto-fix + auto-merge switches", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const taskId = await seedTaskWithLinkedMR(apiClient, seedData, "MR automation section");
    await openTask(testPage, taskId);
    const controls = await openAutomationControls(testPage);

    // AC1: the Automation group (exactly two switches) sits above the
    // existing Review follow-up collapsible.
    const autoFixSwitch = controls.getByRole("switch", {
      name: "Auto-fix CI and address comments",
    });
    const autoMergeSwitch = controls.getByRole("switch", { name: "Auto-merge when ready" });
    const reviewFollowUpTrigger = controls.getByTestId("mr-review-follow-up-trigger");
    await expect(autoFixSwitch).toBeVisible();
    await expect(autoMergeSwitch).toBeVisible();
    const autoFixBox = await autoFixSwitch.boundingBox();
    const triggerBox = await reviewFollowUpTrigger.boundingBox();
    expect(autoFixBox, "auto-fix switch has no layout box").not.toBeNull();
    expect(triggerBox, "review follow-up trigger has no layout box").not.toBeNull();
    expect(autoFixBox!.y).toBeLessThan(triggerBox!.y);

    await expect(autoFixSwitch).not.toBeChecked();
    await expect(autoMergeSwitch).not.toBeChecked();

    // AC2: PATCH persists and a subsequent GET reflects it.
    await autoFixSwitch.click();
    await expect
      .poll(async () => apiClient.getTaskMRAutomationOptions(taskId))
      .toMatchObject({ auto_fix_enabled: true });

    await autoMergeSwitch.click();
    await expect
      .poll(async () => apiClient.getTaskMRAutomationOptions(taskId))
      .toMatchObject({ auto_fix_enabled: true, auto_merge_enabled: true });

    // AC4: default prompt fields are present once auto-fix is on.
    const options = await apiClient.getTaskMRAutomationOptions(taskId);
    expect(options.auto_fix_max_rounds).toBe(10);
    expect(options.effective_auto_fix_prompt.length).toBeGreaterThan(0);
    expect(options.using_default_prompt).toBe(true);

    await testPage.reload();
    await expect(testPage.getByTestId("mr-topbar-button")).toBeVisible({ timeout: 15_000 });
    const reloadedControls = await openAutomationControls(testPage);
    await expect(
      reloadedControls.getByRole("switch", { name: "Auto-fix CI and address comments" }),
    ).toBeChecked();
    await expect(
      reloadedControls.getByRole("switch", { name: "Auto-merge when ready" }),
    ).toBeChecked();
  });

  // AC3 (unknown field / explicit null rejected with 400) is covered at the
  // Go controller level (controller_mr_automation_test.go) — the request
  // helper needed to hit the raw PATCH body from a spec is private, and the
  // ApiClient's typed patch method can't construct an invalid payload.

  test("AC5: a custom prompt override flips using_default_prompt, and resetting restores the default", async ({
    apiClient,
    seedData,
  }) => {
    const taskId = await seedTaskWithLinkedMR(apiClient, seedData, "MR automation prompt override");
    const before = await apiClient.getTaskMRAutomationOptions(taskId);
    const defaultPrompt = before.effective_auto_fix_prompt;

    const withOverride = await apiClient.updateTaskMRAutomationOptions(taskId, {
      auto_fix_prompt_override: "Custom auto-fix instructions for this task",
    });
    expect(withOverride.effective_auto_fix_prompt).toBe(
      "Custom auto-fix instructions for this task",
    );
    expect(withOverride.using_default_prompt).toBe(false);

    const reset = await apiClient.updateTaskMRAutomationOptions(taskId, {
      auto_fix_prompt_override: "",
    });
    expect(reset.effective_auto_fix_prompt).toBe(defaultPrompt);
    expect(reset.using_default_prompt).toBe(true);
  });
});

test.describe("GitLab MR automation options", () => {
  test("desktop hover popover persists lifecycle notification switches and survives reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const taskId = await seedTaskWithLinkedMR(apiClient, seedData, "MR automation desktop");
    await openTask(testPage, taskId);
    const controls = await openAutomationControls(testPage);

    const reviewFollowUp = controls.getByTestId("mr-review-follow-up-trigger");
    await expect(reviewFollowUp).toHaveAttribute("aria-expanded", "false");
    await reviewFollowUp.click();
    await expect(reviewFollowUp).toHaveAttribute("aria-expanded", "true");
    await expect(controls.getByRole("switch", { name: "Your review is requested" })).toBeVisible();
    await expect(controls.getByRole("switch", { name: "MR merged" })).toBeVisible();
    await expect(controls.getByRole("switch", { name: "MR closed without merging" })).toBeVisible();

    await controls.getByTestId("mr-review-requested-help").hover();
    await expect(
      testPage
        .getByRole("tooltip")
        .getByText(
          "Wake the agent when the workspace's connected GitLab account is added as a reviewer. Being re-added after removal counts as a new request; staying assigned across MR updates does not.",
        ),
    ).toBeVisible();
    await controls.getByTestId("mr-terminal-help").hover();
    await expect(
      testPage
        .getByRole("tooltip")
        .getByText("Wake the agent when review work ends. Choose either or both outcomes."),
    ).toBeVisible();

    await controls.getByRole("switch", { name: "Your review is requested" }).click();
    await controls.getByRole("switch", { name: "MR merged" }).click();

    await expect
      .poll(async () => apiClient.getTaskMRAutomationOptions(taskId))
      .toMatchObject({
        prompt_on_review_requested: true,
        prompt_on_merged: true,
      });

    await testPage.reload();
    await expect(testPage.getByTestId("mr-topbar-button")).toBeVisible({ timeout: 15_000 });
    const reloadedControls = await openAutomationControls(testPage);
    const reloadedTrigger = reloadedControls.getByTestId("mr-review-follow-up-trigger");
    // Auto-expands: a switch is already enabled.
    await expect(reloadedTrigger).toHaveAttribute("aria-expanded", "true");
    await expect(
      reloadedControls.getByRole("switch", { name: "Your review is requested" }),
    ).toBeChecked();
    await expect(reloadedControls.getByRole("switch", { name: "MR merged" })).toBeChecked();
  });

  test("desktop hover popover shows a retry banner when the initial load fails", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const taskId = await seedTaskWithLinkedMR(apiClient, seedData, "MR automation load error");
    await interceptLoadFailure(testPage);
    await openTask(testPage, taskId);

    await testPage.getByTestId("mr-topbar-button").hover();
    const controls = testPage.getByTestId("mr-automation-controls");
    await expect(controls).toBeVisible();
    // Visible without opening the collapsible section — the group never
    // auto-expands with no loaded options.
    await expect(controls.getByRole("alert")).toContainText("backend unavailable");
    const retry = controls.getByTestId("mr-automation-retry");
    await expect(retry).toBeVisible();

    await testPage.unroute("**/api/v1/gitlab/tasks/*/mr-automation");
    await retry.click();
    await expect(controls.getByRole("alert")).toHaveCount(0);
    const reviewFollowUp = controls.getByTestId("mr-review-follow-up-trigger");
    await reviewFollowUp.click();
    await expect(
      controls.getByRole("switch", { name: "Your review is requested" }),
    ).not.toBeChecked();
  });
});

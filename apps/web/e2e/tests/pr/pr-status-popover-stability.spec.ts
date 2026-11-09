import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { dwell } from "../../helpers/causal-waits";
import { waitForFiniteAnimations } from "../../helpers/animations";

// @covers AC-INTEGRATIONS-GITHUB-PR-DISCLOSURE-001.1/.2/.3/.4/.5
const cases = [
  { entry: "chip", multiple: false, width: 1280, height: 720 },
  { entry: "topbar", multiple: false, width: 1280, height: 720 },
  { entry: "chip", multiple: true, width: 1000, height: 500 },
  { entry: "topbar", multiple: true, width: 1280, height: 720 },
  { entry: "chip", multiple: false, width: 767, height: 600 },
] as const;
for (const { entry, multiple, width, height } of cases) {
  test(`${entry} ${multiple ? "multiple" : "single"} ${width}px keeps the PR target stationary while feedback arrives`, async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width, height });
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    const task = await apiClient.createTask(seedData.workspaceId, "Stable PR target", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
      repositoryId: seedData.repositoryId,
    });
    const prUrl = "https://github.test/acme/stable/pull/42";
    await apiClient.mockGitHubAssociateTaskPR({
      workspace_id: seedData.workspaceId,
      repository_id: seedData.repositoryId,
      task_id: task.id,
      owner: "acme",
      repo: "stable",
      pr_number: 42,
      pr_url: prUrl,
      pr_title: "Stable PR target",
      head_branch: "feature/stable",
      base_branch: "main",
      author_login: "",
      state: "open",
      checks_state: "pending",
      checks_total: 24,
      checks_passing: 0,
    });
    if (multiple) {
      await apiClient.mockGitHubAssociateTaskPR({
        workspace_id: seedData.workspaceId,
        repository_id: seedData.repositoryId,
        task_id: task.id,
        owner: "acme",
        repo: "stable",
        pr_number: 43,
        pr_url: "https://github.test/acme/stable/pull/43",
        pr_title: "Second PR",
        head_branch: "feature/second",
        base_branch: "main",
        state: "open",
        checks_state: "success",
        checks_total: 1,
        checks_passing: 1,
      });
    }
    await apiClient.mockGitHubSeedPRFeedback({
      owner: "acme",
      repo: "stable",
      pr_number: 42,
      checks: Array.from({ length: 24 }, (_, index) => ({
        name: `Workflow ${index + 1} / test`,
        status: "in_progress",
        html_url: "",
      })),
    });
    let gate = Promise.withResolvers<void>();
    await testPage.route(
      (url) => url.pathname === "/api/v1/github/prs/acme/stable/42",
      async (route) => {
        await gate.promise;
        await route.continue();
      },
    );
    try {
      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      const trigger = entry === "chip" ? session.prStatusChip() : session.prTopbarButton();
      await trigger.hover();
      const frame = testPage.locator('[data-slot="popover-content"]').filter({
        has: testPage.getByTestId("pr-popover-pr-link"),
      });
      const link = frame.getByTestId("pr-popover-pr-link");
      await expect(link).toBeVisible();
      await waitForFiniteAnimations(frame);
      await testPage.screenshot({ path: test.info().outputPath("stable-loading.png") });
      const before = await link.boundingBox();
      const frameBefore = await frame.boundingBox();
      expect(before).not.toBeNull();
      expect(frameBefore).not.toBeNull();
      await frame.hover();
      const response = testPage.waitForResponse(
        (res) => new URL(res.url()).pathname === "/api/v1/github/prs/acme/stable/42",
      );
      gate.resolve();
      await response;
      await expect(frame.getByTestId("pr-workflow-row")).toHaveCount(24);
      const after = await link.boundingBox();
      expect(
        Math.abs(after!.y - before!.y),
        "PR link must not move during loading",
      ).toBeLessThanOrEqual(1);
      const frameAfter = await frame.boundingBox();
      expect(Math.abs(frameAfter!.height - frameBefore!.height)).toBeLessThanOrEqual(1);
      expect(Math.abs(frameAfter!.y - frameBefore!.y)).toBeLessThanOrEqual(1);

      // Summary and detailed feedback settle independently. Keep cached rows
      // visible while the late author and comments trigger another feedback read.
      gate = Promise.withResolvers<void>();
      await apiClient.mockGitHubAssociateTaskPR({
        workspace_id: seedData.workspaceId,
        repository_id: seedData.repositoryId,
        task_id: task.id,
        owner: "acme",
        repo: "stable",
        pr_number: 42,
        pr_url: prUrl,
        pr_title: "Stable PR target",
        author_login: "late-author",
        head_branch: "feature/stable",
        base_branch: "main",
        state: "open",
        checks_state: "pending",
        checks_total: 24,
        checks_passing: 0,
        unresolved_review_threads: 3,
      });
      await expect(frame.getByTestId("pr-popover-author")).toHaveText("by late-author");
      await expect(frame.getByTestId("pr-popover-updating")).toBeVisible();
      await expect(frame.getByTestId("pr-workflow-row")).toHaveCount(24);
      await expect(frame.getByTestId("pr-checks-loading")).toHaveCount(0);
      expect(Math.abs((await link.boundingBox())!.y - before!.y)).toBeLessThanOrEqual(1);
      const refreshed = testPage.waitForResponse(
        (res) => new URL(res.url()).pathname === "/api/v1/github/prs/acme/stable/42",
      );
      gate.resolve();
      await refreshed;
      await expect(link).toHaveText("Open PR");
      expect(frameAfter!.y).toBeGreaterThanOrEqual(0);
      expect(frameAfter!.y + frameAfter!.height).toBeLessThanOrEqual(height);
      expect(frameAfter!.x + frameAfter!.width).toBeLessThanOrEqual(width);
      if (width < 768) expect(after!.height).toBeGreaterThanOrEqual(44);
      const body = frame.getByTestId("pr-popover-scroll-body");
      expect(await body.evaluate((element) => element.scrollWidth - element.clientWidth)).toBe(0);
      await link.focus();
      await testPage.mouse.move(0, 0);
      await dwell(
        testPage,
        200,
        "negative-assertion",
        "focus must keep the disclosure open beyond its 150ms pointer close delay",
      );
      await expect(link).toBeFocused();
      await testPage.keyboard.press("Tab");
      await expect(body).toBeFocused();
      await testPage.keyboard.press("PageDown");
      await expect.poll(() => body.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
      expect(Math.abs((await link.boundingBox())!.y - before!.y)).toBeLessThanOrEqual(1);
      await body.evaluate((element) => {
        element.scrollTop = 0;
      });
      await testPage.screenshot({ path: test.info().outputPath("stable-loaded.png") });
      if (multiple) {
        const tabs = frame.getByRole("tab");
        await tabs.filter({ hasText: "#43" }).click();
        await expect(link).toHaveAttribute("href", "https://github.test/acme/stable/pull/43");
        expect(Math.abs((await link.boundingBox())!.y - before!.y)).toBeLessThanOrEqual(1);
        await tabs.filter({ hasText: "#42" }).click();
        await expect(link).toHaveAttribute("href", prUrl);
      }
      await testPage.context().route(prUrl, (route) => route.fulfill({ body: "PR destination" }));
      const opened = testPage.waitForEvent("popup");
      await testPage.mouse.click(before!.x + before!.width / 2, before!.y + before!.height / 2);
      const popup = await opened;
      await expect(popup).toHaveURL(prUrl);
      await popup.close();
      await testPage.keyboard.press("Escape");
      await expect(frame).toBeHidden();
    } finally {
      gate.resolve();
      await testPage.unrouteAll({ behavior: "wait" });
    }
  });
}

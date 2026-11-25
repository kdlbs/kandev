import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

test.describe("Workflow settings compact list on mobile", () => {
  test("uses touch-sized two-line rows that expand by tap and keeps the view choice", async ({
    testPage,
    seedData,
  }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId, { listView: null });

    const card = page.workflowCard(seedData.workflowId);
    const row = page.compactRow(seedData.workflowId);
    const handle = page.dragHandle(seedData.workflowId);
    await expect(row).toBeVisible();
    await expect(handle).toBeVisible();
    expect(await page.listViewToggleGapBelowDivider()).toBeGreaterThanOrEqual(12);

    const [rowBox, handleBox, cardBox] = await Promise.all([
      row.boundingBox(),
      handle.boundingBox(),
      card.boundingBox(),
    ]);
    expect(rowBox!.height).toBeGreaterThanOrEqual(44);
    expect(handleBox!.height).toBeGreaterThanOrEqual(44);
    expect(handleBox!.width).toBeGreaterThanOrEqual(44);
    expect(handleBox!.x + handleBox!.width).toBeLessThanOrEqual(cardBox!.x + cardBox!.width);

    // Two lines: the step list sits below the workflow name.
    const nameBox = await row.getByText("E2E Workflow", { exact: true }).boundingBox();
    const stepBox = await row.getByTestId("workflow-compact-step").first().boundingBox();
    expect(stepBox!.y).toBeGreaterThanOrEqual(nameBox!.y + nameBox!.height - 1);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    ).toBe(false);

    await row.tap();
    await expect(card).toHaveAttribute("data-expanded", "true");
    await expect(card.locator("input").first()).toBeVisible();
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    ).toBe(false);
    await row.tap();
    await expect(card).toHaveAttribute("data-expanded", "false");

    const detailed = page.listViewOption("detailed");
    expect((await detailed.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await detailed.tap();
    await expect(row).toHaveCount(0);
    await testPage.reload();
    await expect(page.listViewOption("detailed")).toHaveAttribute("data-state", "on");
    await expect(page.compactRow(seedData.workflowId)).toHaveCount(0);
  });

  test("aligns the sync status block with the workflow list on mobile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.configureGitLab(seedData.workspaceId);
    const configResponse = await apiClient.rawRequest(
      "POST",
      `/api/v1/workflow-sync/config?workspace_id=${encodeURIComponent(seedData.workspaceId)}`,
      {
        provider: "gitlab",
        project_path: "platform/kandev",
        branch: "main",
        path: ".kandev/workflows",
        interval_seconds: 300,
        poll_enabled: true,
      },
    );
    expect(configResponse.ok).toBe(true);

    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId, { listView: null });
    await expect(testPage.getByTestId("workflow-sync-section")).toBeVisible();
    await expect(page.compactRow(seedData.workflowId)).toBeVisible();

    const edges = await page.syncSectionAndListEdges();
    // The sync status card and its separator span the same width as the workflow rows.
    for (const block of [edges.card, edges.separator]) {
      expect(Math.abs(block.left - edges.list.left)).toBeLessThanOrEqual(1);
      expect(Math.abs(block.right - edges.list.right)).toBeLessThanOrEqual(1);
    }
    // The card clears the section divider, and the view control clears the separator.
    expect(edges.card.top - edges.dividerTop).toBeGreaterThanOrEqual(12);
    expect(edges.toggleTop - edges.separator.bottom).toBeGreaterThanOrEqual(12);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    ).toBe(false);
  });

  test("keeps step names visible beside a long profile label on a narrow screen", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    const { agents } = await apiClient.listAgents();
    const owner = agents.find((agent) =>
      agent.profiles?.some((profile) => profile.id === seedData.agentProfileId),
    );
    const seedProfile = owner?.profiles?.find((profile) => profile.id === seedData.agentProfileId);
    expect(owner).toBeTruthy();
    expect(seedProfile).toBeTruthy();

    const longProfileLabel = "A very long agent profile label that must not hide workflow steps";
    const profile = await apiClient.createAgentProfile(owner!.id, longProfileLabel, {
      model: seedProfile!.model,
    });
    await apiClient.updateWorkflow(seedData.workflowId, { agent_profile_id: profile.id });

    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId, { listView: null });

    const row = page.compactRow(seedData.workflowId);
    const stepStrip = row.getByTestId("workflow-compact-steps");
    const firstStep = row.getByTestId("workflow-compact-step").first();
    const profileBadge = row.getByTestId("workflow-compact-profile");
    await expect(profileBadge).toContainText(longProfileLabel);

    const [stripBox, stepBox, profileBox] = await Promise.all([
      stepStrip.boundingBox(),
      firstStep.boundingBox(),
      profileBadge.boundingBox(),
    ]);
    expect(profileBox!.width).toBeLessThanOrEqual(80);
    expect(stepBox!.x).toBeGreaterThanOrEqual(stripBox!.x);
    expect(stepBox!.x + stepBox!.width).toBeLessThanOrEqual(stripBox!.x + stripBox!.width + 1);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    ).toBe(false);
  });
});

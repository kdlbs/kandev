// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { expect, test } from "../../fixtures/test-base";
import {
  assertLocatorWithinViewportX,
  assertNoDocumentHorizontalOverflow,
} from "../../helpers/layout-assertions";

test("mobile dependency links preserve the task workbench and touch layout", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const predecessor = await apiClient.createTask(
    seedData.workspaceId,
    "Mobile navigation predecessor",
    {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  const target = await apiClient.createTask(seedData.workspaceId, "Mobile navigation target", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
    blocked_by: [predecessor.id],
  });
  const dependent = await apiClient.createTask(
    seedData.workspaceId,
    "Mobile navigation dependent",
    {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      blocked_by: [target.id],
    },
  );

  await testPage.goto(`/t/${target.id}`);
  const taskTitle = testPage.getByTestId("mobile-task-picker-trigger");
  await expect(taskTitle).toContainText(target.title);

  const documentRequests: string[] = [];
  const onRequest = (request: import("@playwright/test").Request) => {
    if (
      request.isNavigationRequest() &&
      request.frame() === testPage.mainFrame() &&
      request.resourceType() === "document"
    ) {
      documentRequests.push(request.url());
    }
  };
  testPage.on("request", onRequest);

  try {
    await testPage.evaluate(() => {
      (window as Window & { __taskLinkDocumentSentinel?: string }).__taskLinkDocumentSentinel =
        "mobile-dependency-links-stay-in-spa";
    });

    const chip = testPage.getByTestId("task-dependency-chip");
    await chip.tap();
    const drawer = testPage.getByTestId("task-dependency-chip-drawer");
    await expect(drawer).toBeVisible();
    await expect(drawer.locator("[class*='overflow-y-auto']")).toHaveCount(1);
    await assertLocatorWithinViewportX(drawer, "dependency drawer");
    await assertNoDocumentHorizontalOverflow(testPage, "dependency drawer");

    const entries = drawer.getByTestId("task-dependency-entry");
    await expect(entries).toHaveCount(2);
    for (const entry of await entries.all()) {
      const box = await entry.boundingBox();
      expect(box, "dependency entry must have geometry").not.toBeNull();
      expect(box?.height ?? 0, "dependency entry touch target").toBeGreaterThanOrEqual(44);
      await assertLocatorWithinViewportX(entry, "dependency entry");
    }

    const blockedBy = entries.filter({ hasText: predecessor.title });
    await expect(blockedBy).toHaveAttribute("href", `/t/${predecessor.id}`);
    await blockedBy.tap();
    await expect(testPage).toHaveURL(new RegExp(`/t/${predecessor.id}$`));
    await expect(testPage.getByTestId("mobile-task-picker-trigger")).toContainText(
      predecessor.title,
    );
    await expect(testPage.getByTestId("task-dependency-chip-drawer")).toHaveCount(0);
    expect(documentRequests).toHaveLength(0);
    expect(
      await testPage.evaluate(
        () =>
          (window as Window & { __taskLinkDocumentSentinel?: string }).__taskLinkDocumentSentinel,
      ),
    ).toBe("mobile-dependency-links-stay-in-spa");

    await testPage.goBack();
    await expect(testPage).toHaveURL(new RegExp(`/t/${target.id}$`));
    await expect(testPage.getByTestId("mobile-task-picker-trigger")).toContainText(target.title);
    expect(documentRequests).toHaveLength(0);

    await chip.tap();
    const reopenedDrawer = testPage.getByTestId("task-dependency-chip-drawer");
    await expect(reopenedDrawer).toBeVisible();
    const blocks = reopenedDrawer
      .getByTestId("task-dependency-entry")
      .filter({ hasText: dependent.title });
    await expect(blocks).toHaveAttribute("href", `/t/${dependent.id}`);
    await blocks.tap();

    await expect(testPage).toHaveURL(new RegExp(`/t/${dependent.id}$`));
    await expect(testPage.getByTestId("mobile-task-picker-trigger")).toContainText(dependent.title);
    await expect(testPage.getByTestId("task-dependency-chip-drawer")).toHaveCount(0);
    expect(documentRequests).toHaveLength(0);
    expect(
      await testPage.evaluate(
        () =>
          (window as Window & { __taskLinkDocumentSentinel?: string }).__taskLinkDocumentSentinel,
      ),
    ).toBe("mobile-dependency-links-stay-in-spa");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile dependency navigation");
  } finally {
    testPage.off("request", onRequest);
  }
});

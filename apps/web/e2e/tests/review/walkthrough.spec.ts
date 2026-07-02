import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { Page, Locator } from "@playwright/test";

/**
 * Seed a task whose agent emits a 5-step code walkthrough spanning three changed
 * files plus one unchanged file (the `walkthrough-basic` mock-agent scenario).
 */
async function seedWalkthroughTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Walkthrough E2E",
    seedData.agentProfileId,
    {
      description: "/e2e:walkthrough-basic",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  await expect(session.chat.getByText("walkthrough-basic complete", { exact: false })).toBeVisible({
    timeout: 45_000,
  });
  return session;
}

/** Open the diff-anchored tour via the launcher and return the inline card. */
async function openReviewWalkthrough(testPage: Page): Promise<Locator> {
  const launcher = testPage.getByTestId("walkthrough-launcher");
  await expect(launcher).toBeVisible({ timeout: 30_000 });
  await testPage.getByTestId("walkthrough-launcher-review").click();
  const overlay = testPage.getByTestId("walkthrough-overlay");
  await expect(overlay).toBeVisible({ timeout: 30_000 });
  return overlay;
}

test.describe("Code walkthrough (line-anchored)", () => {
  test.describe.configure({ retries: 2, timeout: 120_000 });

  test("diff-anchored card walks multiple steps across different files", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData);
    const overlay = await openReviewWalkthrough(testPage);
    const header = overlay.getByTestId("walkthrough-step-header");
    const body = overlay.getByTestId("walkthrough-step-body");

    await expect(header).toContainText("Step 1 / 5");
    await expect(body).toContainText("Step 1");
    await expect(overlay.getByTestId("walkthrough-prev")).toBeDisabled();

    // Step 2 — still file A, second line.
    await overlay.getByTestId("walkthrough-next").click();
    await expect(header).toContainText("Step 2 / 5");
    await expect(body).toContainText("WALKTHROUGH_CHANGE_A");

    // Step 3 — re-anchors to a different file (B).
    await overlay.getByTestId("walkthrough-next").click();
    await expect(header).toContainText("Step 3 / 5");
    await expect(body).toContainText("WALKTHROUGH_CHANGE_B");

    // Step 4 — another file (C).
    await overlay.getByTestId("walkthrough-next").click();
    await expect(header).toContainText("Step 4 / 5");
    await expect(body).toContainText("WALKTHROUGH_CHANGE_C");

    // Back down the tour.
    await overlay.getByTestId("walkthrough-prev").click();
    await expect(header).toContainText("Step 3 / 5");
  });

  test("ask box offers Add (queue) and Run (ask now) actions", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData);
    const overlay = await openReviewWalkthrough(testPage);

    await overlay.getByRole("textbox").fill("Why does this line exist?");
    await expect(overlay.getByRole("button", { name: "Add" })).toBeEnabled();
    await expect(overlay.getByRole("button", { name: "Run" })).toBeEnabled();
    await overlay.getByRole("button", { name: "Run" }).click();
    await expect(overlay.getByRole("textbox")).toHaveValue("");
  });

  test("close dismisses the tour", async ({ testPage, apiClient, seedData }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData);
    const overlay = await openReviewWalkthrough(testPage);
    await overlay.getByTestId("walkthrough-close").click();
    await expect(overlay).toBeHidden({ timeout: 5_000 });
  });

  test("editor-mode floating window walks through an unchanged file", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData);

    await expect(testPage.getByTestId("walkthrough-launcher")).toBeVisible({ timeout: 30_000 });
    await testPage.getByTestId("walkthrough-launcher-editor").click();

    const floating = testPage.getByTestId("walkthrough-floating");
    await expect(floating).toBeVisible({ timeout: 30_000 });

    // Advance to the final step, which targets a file that did NOT change.
    for (let i = 0; i < 4; i++) {
      await floating.getByTestId("walkthrough-next").click();
    }
    await expect(floating.getByTestId("walkthrough-step-header")).toContainText("Step 5 / 5");
    await expect(floating.getByTestId("walkthrough-step-body")).toContainText(
      "WALKTHROUGH_UNCHANGED",
    );

    // The unchanged file was opened in an editor tab (current state).
    await expect(
      testPage.locator('.dv-default-tab:has-text("walkthrough_unchanged.txt")'),
    ).toBeVisible({ timeout: 15_000 });
  });
});

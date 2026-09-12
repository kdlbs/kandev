import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

function mcpWrite(content: string): string {
  const escaped = JSON.stringify(content).slice(1, -1);
  return `e2e:mcp:kandev:create_task_plan_kandev({"task_id":"{task_id}","content":"${escaped}"})`;
}

function planRevisionScript(): string {
  return [
    'e2e:thinking("Seeding mobile plan revisions...")',
    mcpWrite("Mobile draft A"),
    "e2e:delay(2500)",
    mcpWrite("Mobile draft B"),
    'e2e:message("Mobile revisions seeded.")',
  ].join("\n");
}

async function seedMobilePlanTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile plan restore confirmation",
    seedData.agentProfileId,
    {
      description: planRevisionScript(),
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.chat.getByText("Mobile revisions seeded.", { exact: true })).toBeVisible({
    timeout: 45_000,
  });
  await session.waitForChatIdle({ timeout: 45_000 });

  await session.togglePlanMode();
  await testPage.getByRole("navigation").getByRole("button", { name: "Plan", exact: true }).tap();
  await expect(session.planPanel).toBeVisible({ timeout: 10_000 });
  await expect(session.planPanel).toContainText("Mobile draft B", { timeout: 15_000 });
  return session;
}

test.describe("Mobile plan restore confirmation", () => {
  test("hands history off to a restore sheet and creates a new head revision", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);
    let nativeDialogOpened = false;
    testPage.on("dialog", async (dialog) => {
      nativeDialogOpened = true;
      await dialog.dismiss();
    });

    const session = await seedMobilePlanTask(testPage, apiClient, seedData);
    await session.rewindButton().tap();
    await expect(session.revisionsPopover()).toBeVisible({ timeout: 5_000 });
    await expect(session.revisionRows()).toHaveCount(2, { timeout: 10_000 });

    const row = session.revisionRow(1);
    await session.revertButton(row).tap();

    const inlineConfirmation = testPage.getByTestId("mobile-action-confirmation");
    await expect(inlineConfirmation).toBeVisible();
    await expect(session.revisionsPopover()).toBeHidden();
    await expect(testPage.getByRole("dialog")).toHaveAttribute("data-slot", "drawer-content");
    await expect(inlineConfirmation).toContainText("v1");
    await expect(session.revertConfirmPopover()).toHaveCount(0);

    const restoreBox = await inlineConfirmation
      .getByTestId("plan-revision-restore-confirm")
      .boundingBox();
    expect(restoreBox).not.toBeNull();
    expect(restoreBox!.height).toBeGreaterThanOrEqual(44);
    expect(
      await testPage.evaluate(() => {
        const root = document.scrollingElement ?? document.documentElement;
        return root.scrollWidth <= root.clientWidth + 1;
      }),
    ).toBe(true);

    await inlineConfirmation.getByRole("button", { name: "Cancel" }).tap();
    await expect(inlineConfirmation).toBeHidden();
    await expect(session.rewindButton()).toBeFocused();
    await session.rewindButton().tap();
    await expect(session.revertButton(row)).toBeVisible();

    await session.revertButton(row).tap();
    await inlineConfirmation.getByTestId("plan-revision-restore-confirm").tap();
    await expect(session.planPanel).toContainText("Mobile draft A", { timeout: 15_000 });
    expect(nativeDialogOpened).toBe(false);

    await session.openRewind();
    await expect(session.revisionRows()).toHaveCount(3, { timeout: 10_000 });
    await expect(session.revisionRow(3).getByTestId("plan-revision-current-badge")).toBeVisible();
    await expect(session.revisionRow(3).getByTestId("plan-revision-revert-marker")).toBeVisible();
  });
});

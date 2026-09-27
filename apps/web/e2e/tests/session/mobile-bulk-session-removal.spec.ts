import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { expectContentSizedBottomConfirmation } from "../../helpers/mobile-confirmations";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

test.describe("mobile: bulk session removal", () => {
  test("removes other conversations, then all, through the hosted Sessions sheet", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile bulk session removal",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await expect
      .poll(async () => (await apiClient.listTaskSessions(task.id)).sessions[0]?.state, {
        timeout: 30_000,
      })
      .toMatch(/COMPLETED|WAITING_FOR_INPUT/);
    const primaryId = (await apiClient.listTaskSessions(task.id)).sessions[0]?.id;
    if (!primaryId) throw new Error("task did not create a session");
    const otherIds = [];
    for (let index = 0; index < 2; index += 1) {
      const other = await apiClient.seedTaskSession(task.id, {
        state: "WAITING_FOR_INPUT",
        agentProfileId: seedData.agentProfileId,
        repositoryId: seedData.repositoryId,
        sessionId: `mobile-bulk-${index}-${task.id}`,
        startedAt: `2026-01-01T00:0${index + 1}:00Z`,
      });
      otherIds.push(other.session_id);
    }

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mobile-sessions-pill").tap();
    const sheet = testPage.getByRole("dialog", { name: "Sessions" });
    await expect(sheet).toBeVisible();
    const sheetId = await sheet.getAttribute("id");
    const primaryActions = sheet
      .getByTestId(`mobile-session-row-${primaryId}`)
      .getByRole("button", { name: "Session actions" });
    await expect(primaryActions).toBeVisible();

    await primaryActions.click();
    await testPage.getByRole("menuitem", { name: "Remove Others" }).tap();
    const confirmation = testPage.getByTestId("mobile-bulk-session-remove-confirmation");
    await expect(confirmation).toContainText("Remove 2 sessions?");
    await expectContentSizedBottomConfirmation(sheet, confirmation);
    await testPage.screenshot({ path: test.info().outputPath("bulk-removal-confirmation.png") });
    await expect(sheet).toHaveAttribute("id", sheetId!);
    expect(
      (await confirmation.getByTestId("mobile-bulk-session-remove-confirm").boundingBox())!.height,
    ).toBeGreaterThanOrEqual(44);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile bulk session removal");
    await confirmation.getByRole("button", { name: "Cancel" }).tap();
    expect((await apiClient.listTaskSessions(task.id)).sessions).toHaveLength(3);

    await primaryActions.click();
    await testPage.getByRole("menuitem", { name: "Remove Others" }).tap();
    await confirmation.getByTestId("mobile-bulk-session-remove-confirm").tap();
    await expect
      .poll(async () => (await apiClient.listTaskSessions(task.id)).sessions.map((s) => s.id), {
        timeout: 15_000,
      })
      .toEqual([primaryId]);
    for (const id of otherIds)
      await expect(sheet.getByTestId(`mobile-session-row-${id}`)).toHaveCount(0);

    await primaryActions.click();
    await testPage.getByRole("menuitem", { name: "Remove All" }).tap();
    await expect(confirmation).toContainText("Remove 1 session?");
    await confirmation.getByTestId("mobile-bulk-session-remove-confirm").tap();
    await expect
      .poll(async () => (await apiClient.listTaskSessions(task.id)).sessions.length, {
        timeout: 15_000,
      })
      .toBe(0);
    await testPage.reload();
    await session.waitForLoad();
    expect((await apiClient.listTaskSessions(task.id)).sessions).toHaveLength(0);
  });
});

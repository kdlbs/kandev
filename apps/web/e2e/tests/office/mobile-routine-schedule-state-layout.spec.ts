import { expect, test } from "../../fixtures/office-fixture";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

test.describe("Mobile routine schedule-state row", () => {
  test("wraps schedule metadata and keeps row actions reachable", async ({
    testPage,
    officeApi,
    officeSeed,
    apiClient,
  }) => {
    const routine = (await officeApi.createRoutine(officeSeed.workspaceId, {
      name: "E2E Mobile Routine With A Long Name For Schedule Status",
    })) as { id: string };
    await apiClient.seedRoutineTrigger({
      routineId: routine.id,
      kind: "cron",
      cronExpression: "0 0 30 2 *",
      enabled: true,
    });

    await testPage.goto("/office/routines");
    const row = testPage.getByTestId(`routine-row-${routine.id}`);
    await expect(row).toBeVisible({ timeout: 10_000 });
    await expect(
      row
        .getByText(/^(Armed|Schedule broken|Event-triggered|No schedule|Schedule unknown)$/)
        .first(),
    ).toBeVisible();

    await assertNoDocumentHorizontalOverflow(testPage, "mobile routine schedule-state row");

    const viewport = testPage.viewportSize();
    const action = row.getByRole("button").last();
    const actionBox = await action.boundingBox();
    expect(viewport).toBeTruthy();
    expect(actionBox).toBeTruthy();
    expect(actionBox!.x + actionBox!.width).toBeLessThanOrEqual(viewport!.width);
  });
});

import { test, expect } from "../../fixtures/office-fixture";
import { waitForHttp } from "../../helpers/causal-waits";

// AC-OFFICE-TRIGGER-WIRE-002.5: the backend accepts a cron trigger create and
// the normalized trigger the web app renders carries a real nextRunAt. No
// adapter unit test can observe this criterion, because it asserts on the
// backend's own 201 and its computed next_run_at (docs/specs/office/system-
// design/routine-trigger-wire-contract-01.md, "Observability"): before the
// wire-contract fix, every cron create through this dialog 400'd, so this is
// the spec that would have caught it.
test.describe("Routine trigger wire contract", () => {
  test("a cron schedule armed through the create-routine dialog is accepted and shown on the row", async ({
    testPage,
    officeSeed: _,
  }) => {
    await testPage.goto("/office/routines");
    await testPage.getByRole("button", { name: "New Routine" }).click();

    await testPage.getByLabel("Name").fill("E2E Wire Contract Routine");
    await testPage
      .getByText("Assignee", { exact: true })
      .locator("..")
      .getByRole("combobox")
      .click();
    await testPage.getByRole("option", { name: "CEO" }).click();
    await testPage.getByRole("button", { name: "Next" }).click();
    await testPage.getByRole("button", { name: "Next" }).click();

    await testPage.getByLabel("Cron Expression").fill("*/5 * * * *");

    const routineCreated = waitForHttp(testPage, "POST", /\/workspaces\/[^/]+\/routines$/, {
      predicate: (r) => r.status() === 201,
    });
    const triggerCreated = waitForHttp(testPage, "POST", /\/routines\/[^/]+\/triggers$/, {
      predicate: (r) => r.status() === 201,
    });
    await testPage.getByRole("button", { name: "Create" }).click();
    const routineResponse = await routineCreated;
    const routineBody = (await routineResponse.json()) as { routine?: { id?: string } };
    const routineId = routineBody.routine?.id;
    expect(routineId).toBeTruthy();
    const triggerResponse = await triggerCreated;
    const triggerBody = (await triggerResponse.json()) as { trigger?: { next_run_at?: string } };
    expect(triggerBody.trigger?.next_run_at).toBeTruthy();

    const row = testPage.getByTestId(`routine-row-${routineId}`);
    await expect(row).toBeVisible({ timeout: 10_000 });
    await expect(row.getByText("*/5 * * * *")).toBeVisible({ timeout: 10_000 });
    await expect(row.getByText(/next in/i)).toBeVisible({ timeout: 10_000 });
  });
});

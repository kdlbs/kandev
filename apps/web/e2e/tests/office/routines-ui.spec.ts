import { test, expect } from "../../fixtures/office-fixture";
import { waitForHttp } from "../../helpers/causal-waits";

test.describe("Routines UI", () => {
  test("routine created via API appears in page", async ({ testPage, officeApi, officeSeed }) => {
    await officeApi.createRoutine(officeSeed.workspaceId, {
      name: "E2E Test Routine",
    });
    await testPage.goto("/office/routines");
    await expect(testPage.getByText("E2E Test Routine")).toBeVisible({ timeout: 10_000 });
  });

  // A paused routine's cron-suppression behavior is covered by Go unit
  // tests; this spec covers only the UI surfaces the frontend owns: the
  // row reflects the paused status, and "Run Now" from either surface is
  // refused with the status-gated toast.
  test("paused routine shows off and refuses run now", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const routine = (await officeApi.createRoutine(officeSeed.workspaceId, {
      name: "E2E Paused Routine",
    })) as { id: string };
    expect(routine.id).toBeTruthy();

    await testPage.goto("/office/routines");
    const row = testPage.getByTestId(`routine-row-${routine.id}`);
    await expect(row).toBeVisible({ timeout: 10_000 });
    await expect(row.getByText("On", { exact: true })).toBeVisible({ timeout: 10_000 });

    const paused = await apiClient.rawRequest("PATCH", `/api/v1/office/routines/${routine.id}`, {
      status: "paused",
    });
    expect(paused.ok).toBe(true);

    await testPage.reload();
    await expect(row).toBeVisible({ timeout: 10_000 });
    await expect(row.getByText("Off", { exact: true })).toBeVisible({ timeout: 10_000 });

    // DropdownMenuContent renders in a portal outside the row's DOM
    // subtree, so the trigger is found scoped to the row but the menu
    // item itself is located page-wide once open.
    const rowRunRefused = waitForHttp(testPage, "POST", /\/routines\/[^/]+\/run$/, {
      predicate: (r) => r.status() === 409,
    });
    await row.getByRole("button").last().click();
    await testPage.getByTestId("routine-run-now").click();
    await rowRunRefused;
    await expect(testPage.getByText(/Cannot run: routine status is paused/i).first()).toBeVisible({
      timeout: 10_000,
    });

    await testPage.goto(`/office/routines/${routine.id}`);
    await expect(testPage.getByText(/E2E Paused Routine/).first()).toBeVisible({ timeout: 10_000 });

    const detailRunRefused = waitForHttp(testPage, "POST", /\/routines\/[^/]+\/run$/, {
      predicate: (r) => r.status() === 409,
    });
    await testPage.getByRole("button", { name: "Run now" }).click();
    await detailRunRefused;
    await expect(testPage.getByText(/Cannot run: routine status is paused/i).first()).toBeVisible({
      timeout: 10_000,
    });
  });
});

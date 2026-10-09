/**
 * Mobile-Chrome interaction and geometry coverage for the routine catch-up
 * policy control. The create-dialog and detail-view components have no
 * mobile-specific branching, so touch interactions prove the same policy
 * behavior on a phone viewport while the overflow assertion protects layout.
 */
import { type Page } from "@playwright/test";
import { expect, test } from "../../fixtures/office-fixture";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

const RETIRED_LABEL = /enqueue.?missed.?with.?cap/i;
const SUMMARIZE_MISSED_LABEL = /Summarize missed.*(once|single)/i;

function catchUpPolicyCombobox(page: Page) {
  return page.getByText("Catch-up policy", { exact: true }).locator("..").getByRole("combobox");
}

function catchUpMaxInput(page: Page) {
  return page.getByText("Catch-up max", { exact: true }).locator("..").getByRole("spinbutton");
}

async function assertMobileCatchUpPolicyToggles(page: Page) {
  const catchUpMax = catchUpMaxInput(page);

  await expect(catchUpPolicyCombobox(page)).toHaveText(SUMMARIZE_MISSED_LABEL);
  await expect(catchUpMax).toHaveValue("25");
  await catchUpMax.fill("42");
  await expect(catchUpMax).toHaveValue("42");

  await catchUpPolicyCombobox(page).tap();
  await page.getByRole("option", { name: "Skip missed", exact: true }).tap();
  await expect(page.getByText("Catch-up max", { exact: true })).toHaveCount(0);

  await catchUpPolicyCombobox(page).tap();
  await page.getByRole("option", { name: SUMMARIZE_MISSED_LABEL, exact: true }).tap();
  await expect(catchUpMaxInput(page)).toHaveValue("42");
  await expect(page.getByText(RETIRED_LABEL)).toHaveCount(0);
}

test.describe("Mobile routine catch-up policy control", () => {
  test("create dialog: catch-up policy control responds to touch input", async ({
    testPage,
    prCapture,
  }) => {
    await testPage.goto("/office/routines");
    await testPage.getByRole("button", { name: "New Routine" }).tap();

    await testPage.getByLabel("Name").fill("E2E Mobile Catch-up Dialog");
    await testPage.getByText("Assignee", { exact: true }).locator("..").getByRole("combobox").tap();
    await testPage.getByRole("option", { name: "CEO", exact: true }).tap();
    await testPage.getByRole("button", { name: "Next" }).tap();
    await testPage.getByRole("button", { name: "Next" }).tap();

    await expect(testPage.getByText("Catch-up policy", { exact: true })).toBeVisible();
    await assertMobileCatchUpPolicyToggles(testPage);
    await assertNoDocumentHorizontalOverflow(testPage);

    await prCapture.screenshot("mobile-create-dialog-catch-up-policy", {
      caption: "Create Routine dialog on a phone viewport with the catch-up policy control",
    });
  });

  test("detail view: catch-up policy control responds to touch input", async ({
    officeApi,
    officeSeed,
    testPage,
    prCapture,
  }) => {
    const routineName = "E2E Mobile Catch-up Detail Toggle";
    const routine = (await officeApi.createRoutine(officeSeed.workspaceId, {
      name: routineName,
    })) as { id: string };
    expect(routine.id).toBeTruthy();

    await testPage.goto(`/office/routines/${routine.id}`);
    await expect(testPage.locator("main input").first()).toHaveValue(routineName, {
      timeout: 10_000,
    });

    await assertMobileCatchUpPolicyToggles(testPage);
    await assertNoDocumentHorizontalOverflow(testPage);

    await prCapture.screenshot("mobile-detail-view-catch-up-policy", {
      caption: "Routine detail view on a phone viewport with the catch-up policy control",
    });
  });
});

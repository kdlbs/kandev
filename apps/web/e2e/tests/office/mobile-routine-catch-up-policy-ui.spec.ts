/**
 * Mobile-Chrome geometry coverage for the routine catch-up policy control
 * (gap 24). The create-dialog and detail-view components have no
 * mobile-specific branching (see routine-catch-up-policy-ui.spec.ts for the
 * desktop coverage of the same control), so this only needs to confirm the
 * control renders inside the phone viewport without horizontal overflow.
 */
import { expect, test } from "../../fixtures/office-fixture";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

test.describe("Mobile routine catch-up policy control", () => {
  test("create dialog: catch-up policy control fits the phone viewport", async ({
    testPage,
    prCapture,
  }) => {
    await testPage.goto("/office/routines");
    await testPage.getByRole("button", { name: "New Routine" }).tap();

    await testPage.getByLabel("Name").fill("E2E Mobile Catch-up Dialog");
    await testPage.getByText("Assignee", { exact: true }).locator("..").getByRole("combobox").tap();
    await testPage.getByRole("option", { name: "CEO" }).tap();
    await testPage.getByRole("button", { name: "Next" }).tap();
    await testPage.getByRole("button", { name: "Next" }).tap();

    await expect(testPage.getByText("Catch-up policy", { exact: true })).toBeVisible();
    await expect(testPage.getByLabel("Catch-up max")).toHaveValue("25");
    await assertNoDocumentHorizontalOverflow(testPage);

    await prCapture.screenshot("mobile-create-dialog-catch-up-policy", {
      caption: "Create Routine dialog on a phone viewport with the catch-up policy control visible",
    });
  });
});

// Guided setup on a phone viewport (matched by the `mobile-chrome` project via
// the mobile- filename prefix).
import { test, expect } from "../../fixtures/test-base";
import { linkToCoordinatorAdd } from "../../../lib/coordinator/links";

const MIN_TOUCH_TARGET_PX = 44;

test.describe("Guided setup on a phone viewport", () => {
  test("shows the step name instead of the list and keeps buttons touch sized", async ({
    testPage,
    seedData,
  }) => {
    await testPage.goto(linkToCoordinatorAdd(seedData.workspaceId));

    await expect(testPage.getByTestId("setup-step-compact")).toContainText(
      "Step 1 of 6: Who runs it",
    );
    await expect(testPage.getByTestId("setup-step-list")).toBeHidden();
    const box = await testPage.getByTestId("setup-next").boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);

    const overflow = await testPage.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
    }));
    expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);

    await testPage.getByLabel("Name").fill("Phone Planner");
    if (await testPage.getByTestId("setup-next").isDisabled()) {
      await testPage.getByTestId("coordinator-agent-profile-picker").click();
      await testPage.getByRole("option").first().click();
      await testPage.getByTestId("executor-profile-selector").first().click();
      await testPage.getByRole("option").first().click();
    }
    await expect(testPage.getByTestId("setup-next")).toBeEnabled();
    await testPage.getByTestId("setup-next").click();
    await expect(testPage.getByTestId("setup-step-compact")).toContainText(
      "Step 2 of 6: What it watches",
    );
  });
});

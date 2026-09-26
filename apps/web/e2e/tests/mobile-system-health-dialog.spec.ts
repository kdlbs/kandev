import { expect, test } from "../fixtures/test-base";
import { MobileKanbanPage } from "../pages/mobile-kanban-page";

const FINAL_ISSUE_ID = "health-23";

function longHealthResponse() {
  return {
    healthy: false,
    issues: Array.from({ length: 24 }, (_, index) => ({
      id: `health-${index}`,
      category: "system_resources",
      title: `System health issue ${index}`,
      message: `The diagnostic message for issue ${index} needs operator attention.`,
      severity: index % 2 === 0 ? "warning" : "error",
      fix_url: "/settings/system/status",
      fix_label: `Fix issue ${index}`,
    })),
    checks: [],
  };
}

test.describe("System health issue dialog on mobile", () => {
  test("keeps issue cards and 44px Fix actions inside the dialog", async ({
    testPage,
    backend,
  }) => {
    test.setTimeout(60_000);
    await testPage.route(`${backend.baseUrl}/api/v1/system/health`, (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(longHealthResponse()),
      }),
    );

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileMenuButton.click();
    await testPage.getByRole("button", { name: "Health issues" }).tap();

    const dialog = testPage.getByTestId("system-health-issues-dialog");
    await expect(dialog).toBeVisible();
    await dialog.evaluate(async (element) => {
      const animations = element.getAnimations({ subtree: true }).filter((animation) => {
        if (animation.playState !== "running") {
          return false;
        }

        const iterations = animation.effect?.getComputedTiming().iterations;
        return typeof iterations === "number" && Number.isFinite(iterations);
      });

      await Promise.all(animations.map((animation) => animation.finished.catch(() => undefined)));
    });
    const body = dialog.getByTestId("system-health-issues-body");
    const title = dialog.locator('[data-slot="dialog-title"]');
    const count = dialog.locator('[data-slot="dialog-description"]');
    const close = dialog.locator('[data-slot="dialog-close"]');
    const finalIssue = body.getByTestId(`system-health-issue-${FINAL_ISSUE_ID}`);
    const finalFix = finalIssue.getByRole("button", { name: "Fix issue 23" });
    await expect(finalIssue).toHaveCount(1);

    const scrollable = await body.evaluate((element) => ({
      scrollHeight: element.scrollHeight,
      clientHeight: element.clientHeight,
    }));
    expect(scrollable.scrollHeight).toBeGreaterThan(scrollable.clientHeight);

    const viewportHeight = await testPage.evaluate(() => window.innerHeight);
    const viewportWidth = await testPage.evaluate(() => window.innerWidth);
    const dialogBox = await dialog.boundingBox();
    const titleBox = await title.boundingBox();
    const countBox = await count.boundingBox();
    const closeBox = await close.boundingBox();
    expect(dialogBox).not.toBeNull();
    expect(titleBox).not.toBeNull();
    expect(countBox).not.toBeNull();
    expect(closeBox).not.toBeNull();
    expect(dialogBox!.x).toBeGreaterThanOrEqual(0);
    expect(dialogBox!.x + dialogBox!.width).toBeLessThanOrEqual(viewportWidth);
    expect(dialogBox!.y).toBeGreaterThanOrEqual(0);
    expect(dialogBox!.y + dialogBox!.height).toBeLessThanOrEqual(viewportHeight);
    expect(titleBox!.y).toBeGreaterThanOrEqual(dialogBox!.y);
    expect(titleBox!.y + titleBox!.height).toBeLessThanOrEqual(dialogBox!.y + dialogBox!.height);
    expect(countBox!.y).toBeGreaterThanOrEqual(dialogBox!.y);
    expect(countBox!.y + countBox!.height).toBeLessThanOrEqual(dialogBox!.y + dialogBox!.height);
    expect(closeBox!.y).toBeGreaterThanOrEqual(dialogBox!.y);
    expect(closeBox!.y + closeBox!.height).toBeLessThanOrEqual(dialogBox!.y + dialogBox!.height);

    await body.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    await expect.poll(() => body.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
    await finalIssue.scrollIntoViewIfNeeded();
    await expect(finalFix).toBeVisible();

    const finalIssueBox = await finalIssue.boundingBox();
    const finalFixBox = await finalFix.boundingBox();
    expect(finalIssueBox).not.toBeNull();
    expect(finalFixBox).not.toBeNull();
    expect(finalIssueBox!.y).toBeGreaterThanOrEqual(dialogBox!.y);
    expect(finalIssueBox!.y + finalIssueBox!.height).toBeLessThanOrEqual(
      dialogBox!.y + dialogBox!.height,
    );
    expect(finalFixBox!.height).toBeGreaterThanOrEqual(44);
    expect(finalFixBox!.y).toBeGreaterThanOrEqual(finalIssueBox!.y);
    expect(finalFixBox!.y + finalFixBox!.height).toBeLessThanOrEqual(
      finalIssueBox!.y + finalIssueBox!.height,
    );
    await expect
      .poll(async () =>
        finalFix.evaluate((element) => {
          const rect = element.getBoundingClientRect();
          const hit = document.elementFromPoint(
            rect.left + rect.width / 2,
            rect.top + rect.height / 2,
          );
          return hit === element || element.contains(hit);
        }),
      )
      .toBe(true);

    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await finalFix.tap();
    await expect(dialog).not.toBeVisible();
    await expect(testPage).toHaveURL(/\/settings\/system\/status$/);
  });
});

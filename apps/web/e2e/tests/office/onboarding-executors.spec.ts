import { test, expect } from "../../fixtures/test-base";

test.describe("First-run executor discovery", () => {
  test("shows six informational executor cards and keeps tour controls reachable", async ({
    testPage,
  }) => {
    await testPage.addInitScript(() => {
      localStorage.removeItem("kandev.onboarding.completed");
    });
    await testPage.setViewportSize({ width: 1280, height: 480 });
    await testPage.goto("/");

    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: "Next" }).click();
    await expect(dialog.getByRole("heading", { name: "Executors" })).toBeVisible();

    const cards = dialog.locator('[data-testid^="onboarding-executor-card-"]');
    await expect(cards).toHaveCount(6);
    expect(
      await cards.evaluateAll((elements) =>
        elements.map((element) => element.getAttribute("data-executor-id")),
      ),
    ).toEqual(["worktree", "local", "local_docker", "ssh", "sprites", "k8s"]);

    const grid = dialog.getByTestId("onboarding-executor-grid");
    expect(
      await grid.evaluate(
        (element) => getComputedStyle(element).gridTemplateColumns.trim().split(/\s+/).length,
      ),
    ).toBe(2);

    await expect(dialog.getByTestId("onboarding-executor-card-worktree")).toContainText(
      "Recommended for existing Git repositories",
    );
    await expect(dialog.getByTestId("onboarding-executor-card-local")).toContainText(
      "selected folder",
    );
    await expect(dialog.getByTestId("onboarding-executor-card-local_docker")).toContainText(
      "Docker daemon",
    );
    const guideLink = dialog.getByRole("link", { name: "View executor guide" });
    await expect(guideLink).toHaveAttribute("href", "https://kandev.ai/docs/executors");
    await expect(guideLink).toHaveAttribute("target", "_blank");

    const body = dialog.getByTestId("onboarding-executor-body");
    const [dialogBox, scrollState] = await Promise.all([
      dialog.boundingBox(),
      body.evaluate((element) => ({
        clientHeight: element.clientHeight,
        scrollHeight: element.scrollHeight,
      })),
    ]);
    expect(dialogBox).not.toBeNull();
    expect(dialogBox!.y).toBeGreaterThanOrEqual(0);
    expect(dialogBox!.y + dialogBox!.height).toBeLessThanOrEqual(480);
    expect(scrollState.scrollHeight).toBeGreaterThan(scrollState.clientHeight);

    await body.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    await expect(guideLink).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Next" })).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Skip" })).toBeVisible();

    await dialog.getByRole("button", { name: "Next" }).click();
    await expect(dialog.getByRole("heading", { name: "Agentic Workflows" })).toBeVisible();
  });
});

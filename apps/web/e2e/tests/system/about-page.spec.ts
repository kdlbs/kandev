import { test, expect } from "../../fixtures/test-base";

test.describe("System About page", () => {
  test("renders version metadata rows and the GitHub link", async ({ testPage }) => {
    test.setTimeout(30_000);

    await testPage.goto("/settings/system/about");

    await expect(testPage.getByTestId("system-page-title")).toHaveText("About");
    await expect(testPage.getByTestId("system-about-card")).toBeVisible();

    // Each row must render *some* value (could be "dev" / "unknown" — just non-empty).
    const ids = [
      "system-about-version",
      "system-about-commit",
      "system-about-build-time",
      "system-about-os",
      "system-about-arch",
    ];
    for (const id of ids) {
      const value = (await testPage.getByTestId(id).innerText()).trim();
      expect(value.length).toBeGreaterThan(0);
    }

    const githubLink = testPage.getByTestId("system-about-github-link");
    await expect(githubLink).toBeVisible();
    await expect(githubLink).toHaveAttribute("href", "https://github.com/kdlbs/kandev");
  });
});

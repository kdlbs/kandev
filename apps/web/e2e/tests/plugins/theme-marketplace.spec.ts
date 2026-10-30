import { createServer } from "node:http";
import { test, expect } from "../../fixtures/test-base";
import { buildThemeRelease } from "./theme-package-fixture";

test("installs a verified theme from a real registry source", async ({ testPage, apiClient }) => {
  const { archive, digest } = buildThemeRelease("theme-study", "1.0.0");
  let url = "";
  const server = createServer((req, res) => {
    if (req.url === "/theme.tar.gz") {
      res.end(archive);
      return;
    }
    res.setHeader("Content-Type", "application/json");
    res.end(
      JSON.stringify({
        schema_version: 1,
        plugins: [
          {
            id: "theme-study",
            kind: "theme",
            name: "Monokai study",
            description: "Two demonstration palettes",
            version: "1.0.0",
            author: "Kandev contributors",
            license: "MIT",
            categories: ["theme"],
            package_url: url + "/theme.tar.gz",
            package_sha256: digest,
          },
        ],
      }),
    );
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  url = `http://127.0.0.1:${(server.address() as { port: number }).port}`;
  let sourceId = "";
  try {
    const response = await apiClient.rawRequest("POST", "/api/plugins/marketplace/sources", {
      name: "Theme fixture",
      url: url + "/index.json",
    });
    expect(response.ok).toBe(true);
    const source = await response.json();
    sourceId = source.id ?? source.source?.id;
    await testPage.goto("/settings/plugins?tab=themes");
    const listing = testPage
      .getByTestId("theme-marketplace")
      .locator("article")
      .filter({ hasText: "Monokai study" });
    await expect(listing).toBeVisible();
    await listing.getByRole("button", { name: "Review theme" }).click();
    await testPage.getByTestId("theme-review").click();
    await expect(testPage.getByTestId("theme-review-content")).toContainText("Night");
    await testPage.getByTestId("theme-confirm").click();
    await expect(testPage.getByTestId("theme-installed")).toBeVisible();
    expect(await testPage.evaluate(() => localStorage.getItem("theme"))).not.toBe(
      "theme:theme-study/night",
    );
  } finally {
    await apiClient.rawRequest("DELETE", "/api/plugins/theme-study").catch(() => undefined);
    if (sourceId)
      await apiClient
        .rawRequest("DELETE", `/api/plugins/marketplace/sources/${sourceId}`)
        .catch(() => undefined);
    await new Promise<void>((resolve) => server.close(() => resolve()));
  }
});

import { updateThemeFromMarketplace } from "./theme-marketplace-update-flow";

test("updates a theme from Installed with verified release identity", async ({
  testPage,
  apiClient,
}) => {
  await updateThemeFromMarketplace(testPage, apiClient);
});

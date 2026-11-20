import { devices } from "@playwright/test";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { setupAdmin } from "../../helpers/auth";
import { repeatTokenCreation } from "./api-token-repeat-helpers";

test.describe("repeat API token creation on phone", () => {
  test.beforeAll(async ({ backend }) => {
    console.log(`Owned fixture root: ${backend.tmpDir}`);
    await backend.restart({
      KANDEV_FEATURES_AUTH: "true",
      KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "tokens-mobile.db"),
    });
  });
  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("creates a second token after closing the first result", async ({ browser, backend }) => {
    const context = await browser.newContext({
      ...devices["Pixel 5"],
      baseURL: backend.frontendUrl,
    });
    try {
      await setupAdmin(context, backend.baseUrl, {
        email: "tokens-mobile@e2e.dev",
        password: "adminpass123",
      });
      await repeatTokenCreation(await context.newPage(), true);
    } finally {
      await context.close();
    }
  });
});

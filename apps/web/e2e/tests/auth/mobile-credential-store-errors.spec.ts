import { devices } from "@playwright/test";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { setupAdmin } from "../../helpers/auth";
import {
  proveCredentialStoreContinuity,
  type CredentialStoreAccount,
} from "./credential-store-errors-helpers";

const ACCOUNT: CredentialStoreAccount = {
  email: "credential-store-mobile@e2e.dev",
  password: "adminpass123",
};

test.describe("authentication storage continuity on phone", () => {
  test.beforeAll(async ({ backend }) => {
    console.log(`Owned fixture root: ${backend.tmpDir}`);
    await backend.restart({
      KANDEV_FEATURES_AUTH: "true",
      KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "credential-store-mobile.db"),
    });
  });
  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("keeps the signed-in account through a 503 and recovers on the same session", async ({
    browser,
    backend,
  }) => {
    const context = await browser.newContext({
      ...devices["Pixel 5"],
      baseURL: backend.frontendUrl,
    });
    try {
      await setupAdmin(context, backend.baseUrl, ACCOUNT);
      await proveCredentialStoreContinuity(await context.newPage(), ACCOUNT);
    } finally {
      await context.close();
    }
  });
});

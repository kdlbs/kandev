// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { test } from "../../fixtures/test-base";
import { runManagedRuntimeGitRestart } from "./managed-runtime-git-restart-helpers";

test.describe("Managed runtime restart in phone Changes", () => {
  test("recovers live membership and settled detail after a startup retry", async ({
    testPage,
    apiClient,
    backend,
    seedData,
    prCapture,
  }, testInfo) => {
    test.skip(process.platform === "win32", "the deterministic npx fixture is a POSIX script");
    test.setTimeout(240_000);
    await runManagedRuntimeGitRestart({
      page: testPage,
      apiClient,
      backend,
      seedData,
      prCapture,
      testInfo,
      mobile: true,
    });
  });
});

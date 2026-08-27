import { expect, test } from "../../fixtures/test-base";

test.describe.serial("environment browser title prefixes", () => {
  test("uses Debug for a diagnostics-only production profile", async ({ backend, testPage }) => {
    await backend.restart({
      KANDEV_E2E_MOCK: "",
      KANDEV_DEBUG_DEV_MODE: "",
      KANDEV_DEBUG_PPROF_ENABLED: "true",
      KANDEV_WEB_TITLE_PREFIX: "Debug",
    });

    try {
      await testPage.goto("/");
      await expect(testPage).toHaveTitle("Debug Kandev");
    } finally {
      await backend.restart();
    }
  });

  test("uses Dev for the development profile", async ({ backend, testPage }) => {
    await backend.restart({ KANDEV_E2E_MOCK: "", KANDEV_DEBUG_DEV_MODE: "true" });

    try {
      await testPage.goto("/");
      await expect(testPage).toHaveTitle("Dev Kandev");
    } finally {
      await backend.restart();
    }
  });

  test("uses Preview for an explicitly configured preview backend", async ({
    backend,
    testPage,
  }) => {
    await backend.restart({ KANDEV_WEB_TITLE_PREFIX: "Preview" });

    try {
      await testPage.goto("/");
      await expect(testPage).toHaveTitle("Preview Kandev");
    } finally {
      await backend.restart();
    }
  });
});

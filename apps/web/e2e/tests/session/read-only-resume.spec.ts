import { test } from "../../fixtures/test-base";
import { verifyReadOnlyResume } from "./read-only-resume-helpers";

test("Resume remains available after automatic read-only workspace recovery", async ({
  testPage,
  apiClient,
  seedData,
  backend,
  prCapture,
}, testInfo) => {
  test.setTimeout(150_000);
  await verifyReadOnlyResume(testPage, apiClient, seedData, testInfo, {
    tmpDir: backend.tmpDir,
    capture: prCapture,
    mobile: false,
    restart: backend.restart,
  });
});

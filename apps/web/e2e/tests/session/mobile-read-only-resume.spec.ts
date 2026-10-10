import { test } from "../../fixtures/test-base";
import { verifyReadOnlyResume } from "./read-only-resume-helpers";

test("touch Resume works after read-only workspace fallback without a stopped-agent card", async ({
  testPage,
  apiClient,
  seedData,
  backend,
  prCapture,
}) => {
  test.setTimeout(150_000);
  await verifyReadOnlyResume(testPage, apiClient, seedData, {
    tmpDir: backend.tmpDir,
    capture: prCapture,
    mobile: true,
    restart: backend.restart,
  });
});

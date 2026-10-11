import { test } from "../../fixtures/test-base";
import { verifyQuickChatResumeRecovery } from "./quick-chat-resume-recovery-helpers";

test("existing Resume recovers interrupted repository-free Quick Chat", async ({
  testPage,
  apiClient,
  backend,
  prCapture,
}) => {
  test.setTimeout(120_000);
  await verifyQuickChatResumeRecovery(testPage, apiClient, backend.tmpDir, false, prCapture);
});

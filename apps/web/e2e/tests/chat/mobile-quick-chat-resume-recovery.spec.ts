import { test } from "../../fixtures/test-base";
import { verifyQuickChatResumeRecovery } from "./quick-chat-resume-recovery-helpers";

test("existing Resume recovers interrupted Quick Chat on a phone", async ({
  testPage,
  apiClient,
  backend,
}) => {
  test.setTimeout(120_000);
  await verifyQuickChatResumeRecovery(testPage, apiClient, backend.tmpDir, true);
});

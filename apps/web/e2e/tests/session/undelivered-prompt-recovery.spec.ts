import { test } from "../../fixtures/test-base";
import { verifyNativeResumeClearsRecoveryNotice } from "../../helpers/native-resume-notice";

// @covers AC-PLATFORM-UNDELIVERED-PROMPT-001.2, AC-PLATFORM-UNDELIVERED-PROMPT-001.4, AC-PLATFORM-UNDELIVERED-PROMPT-001.5
test("Resume retires two SQL-only prompts and preserves the conversation", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(120_000);
  await verifyNativeResumeClearsRecoveryNotice(
    testPage,
    apiClient,
    seedData,
    backend.tmpDir,
    "absent",
  );
});

import { test } from "../../fixtures/test-base";
import { exerciseSSHAccess } from "../../helpers/ssh-access-setup";
// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.10, 004.11
test("explicit access setup clears secrets and replaces verified enrollment (UI fixtures)", async ({
  backend,
  testPage,
}) => {
  const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
  try {
    await exerciseSSHAccess(testPage, backend.baseUrl, false);
  } finally {
    await testPage.close();
    await release();
  }
});

// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.16, 004.17, 004.19
import { exerciseSSHTrustReview } from "../../helpers/ssh-trust-review";
test("inline host review shows complete evidence before explicit trust", async ({
  backend,
  testPage,
}) => {
  const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
  try {
    await exerciseSSHTrustReview(testPage, backend.baseUrl, false);
  } finally {
    await testPage.close();
    await release();
  }
});

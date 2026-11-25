import { test } from "../../fixtures/test-base";
import { exerciseSSHIdentityDependencies } from "../../helpers/ssh-identity-dependencies";

// @covers AC-EXECUTORS-SSH-CONNECTIONS-001.6
test("explains blocked identity deletion and opens its connection", async ({
  backend,
  testPage,
  prCapture,
}) => {
  const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
  try {
    await exerciseSSHIdentityDependencies(testPage, backend.baseUrl, true, prCapture);
  } finally {
    await testPage.close();
    await release();
  }
});

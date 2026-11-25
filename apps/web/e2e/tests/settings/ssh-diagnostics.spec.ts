import { test } from "../../fixtures/test-base";
import { exerciseSSHDiagnostics } from "../../helpers/ssh-diagnostics";
// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.18, 004.19
test("SSH setup reports offer remedies without claiming a live login", async ({
  backend,
  testPage,
}) => {
  const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
  try {
    await testPage.setViewportSize({ width: 1440, height: 900 });
    await exerciseSSHDiagnostics(testPage, backend.baseUrl, false);
  } finally {
    await testPage.close();
    await release();
  }
});

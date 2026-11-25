import { test } from "../../fixtures/test-base";
import { exerciseImportedSSHRepair } from "../../helpers/ssh-migration-ui";

// @covers AC-EXECUTORS-SSH-MIGRATION-001.4, AC-EXECUTORS-SSH-MIGRATION-001.6
test("keeps the migrated profile link and opens SSH repair in Access", async ({
  apiClient,
  backend,
  testPage,
}) => {
  await exerciseImportedSSHRepair(testPage, backend, apiClient, false);
});

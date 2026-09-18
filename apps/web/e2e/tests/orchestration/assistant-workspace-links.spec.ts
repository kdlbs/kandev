import { test } from "../../fixtures/test-base";
import { ASSISTANT_ENV } from "../../helpers/personal-assistant";
import { exerciseWorkspaceLinks } from "../../helpers/assistant-workspace-links";
test("owner explicitly grants, forgets and revokes a synthetic linked workspace", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180000);
  await backend.restart(ASSISTANT_ENV);
  await exerciseWorkspaceLinks(testPage, backend, apiClient, seedData);
  await testPage.getByTestId("workspace-grant-card").scrollIntoViewIfNeeded();
  await testPage.screenshot({ path: test.info().outputPath("assistant-workspace-links.png") });
});

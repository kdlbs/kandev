import { test } from "../../fixtures/test-base";
import { ASSISTANT_ENV } from "../../helpers/personal-assistant";
import { exerciseWorkspaceLinks } from "../../helpers/assistant-workspace-links";
test("phone workspace grants preserve receiving-account consent and revocation", async ({
  testPage,
  backend,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180000);
  await backend.restart(ASSISTANT_ENV);
  await exerciseWorkspaceLinks(testPage, backend, apiClient, seedData);
  await testPage.getByTestId("workspace-grant-card").scrollIntoViewIfNeeded();
  await testPage.screenshot({
    path: test.info().outputPath("assistant-workspace-links-phone.png"),
  });
});

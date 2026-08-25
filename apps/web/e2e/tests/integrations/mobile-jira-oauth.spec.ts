import { expect, test } from "../../fixtures/test-base";
import { JiraSettingsPage } from "../../pages/jira-settings-page";

test("Jira OAuth connect is reachable with a touch-sized action", async ({
  testPage,
  seedData,
}) => {
  const settings = new JiraSettingsPage(testPage);
  await settings.gotoWorkspace(seedData.workspaceId);

  await settings.siteInput.fill("https://acme.atlassian.net");
  await settings.selectAuth("OAuth 2.0");

  const connect = testPage.getByTestId("jira-oauth-connect");
  await expect(connect).toBeEnabled();
  const box = await connect.boundingBox();
  expect(box?.height).toBeGreaterThanOrEqual(44);
});

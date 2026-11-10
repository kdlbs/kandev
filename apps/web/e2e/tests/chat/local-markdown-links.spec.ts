import { test } from "../../fixtures/test-base";
import { verifyLocalMarkdownLinks } from "../../helpers/local-markdown-links";
test("opens original local Markdown links and images without publishing", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120000);
  await verifyLocalMarkdownLinks(testPage, apiClient, seedData);
});

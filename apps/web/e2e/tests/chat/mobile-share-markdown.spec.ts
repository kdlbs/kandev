import { test } from "../../fixtures/test-base";
import { verifyShareMarkdownPreview } from "./share-markdown-flow";

test.describe("Mobile share preview", () => {
  test("renders conversation markdown without overflowing the dialog", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    await verifyShareMarkdownPreview(testPage, apiClient, seedData);
  });
});

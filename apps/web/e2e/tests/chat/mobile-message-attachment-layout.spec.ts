import { test } from "../../fixtures/test-base";
import { assertMixedAttachmentLayout } from "../../helpers/mixed-attachment-layout";

test.describe("mobile message attachment layout", () => {
  test("keeps a file attachment compact next to an image preview", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(90_000);
    await assertMixedAttachmentLayout({
      testPage,
      apiClient,
      seedData,
      testInfo,
      taskTitle: "Mobile mixed attachment layout",
    });
  });
});

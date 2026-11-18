import { test } from "../../fixtures/test-base";
import { fileViewerMarketplaceFlow } from "./file-viewer-marketplace-flow";
test("phone viewer marketplace uses an exact removable extension filter", async ({
  testPage,
  apiClient,
}) => {
  await fileViewerMarketplaceFlow(testPage, apiClient);
});

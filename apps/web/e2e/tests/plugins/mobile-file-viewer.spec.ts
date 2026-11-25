import { test } from "../../fixtures/test-base";
import { fileViewerFlow } from "./file-viewer-flow";
test("phone preview requires consent for an external model above the editor limit", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(120000);
  await fileViewerFlow({ testPage, apiClient, seedData, backend, mobile: true });
});

import { test } from "../../fixtures/cursor-cloud";
import { createCloudProfileFromSettings } from "./cursor-cloud-executor-setup-helpers";

test("creates a configured Cursor Cloud executor through settings", async ({
  testPage,
  apiClient,
  backend,
  cursorCloud,
}) => {
  cursorCloud.reset();
  await createCloudProfileFromSettings(testPage, apiClient, backend, false);
});

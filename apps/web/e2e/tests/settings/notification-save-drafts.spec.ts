import { test } from "../../fixtures/test-base";
import { notificationSaveDraftScenario } from "./notification-save-drafts-helpers";

test("preserves later notification edits across acknowledgement, second Save and Reset", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  await notificationSaveDraftScenario({ testPage, apiClient, backend, seedData }, false);
});

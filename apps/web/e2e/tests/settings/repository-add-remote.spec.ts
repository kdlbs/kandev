import { test } from "../../fixtures/test-base";
import { addRemoteRepositoryFromSettings } from "./repository-add-remote-helpers";

test.describe("Add remote repository from settings", () => {
  test("registers a picked provider repository without creating a task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await addRemoteRepositoryFromSettings({ page: testPage, apiClient, seedData });
  });
});

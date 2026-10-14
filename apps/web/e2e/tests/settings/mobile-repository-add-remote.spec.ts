import { test } from "../../fixtures/test-base";
import { addRemoteRepositoryFromSettings } from "./repository-add-remote-helpers";

test.describe("Add remote repository from settings (phone)", () => {
  test("registers a pasted repository URL on a phone viewport", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await addRemoteRepositoryFromSettings({
      page: testPage,
      apiClient,
      seedData,
      mobile: true,
      paste: true,
    });
  });

  test("registers a picked provider repository on a phone viewport", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await addRemoteRepositoryFromSettings({
      page: testPage,
      apiClient,
      seedData,
      mobile: true,
    });
  });
});

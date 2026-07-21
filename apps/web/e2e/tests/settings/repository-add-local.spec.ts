import { test } from "../../fixtures/test-base";
import { addExplicitLocalRepository } from "./repository-add-local-helpers";

test.describe("Add explicit local repository", () => {
  test("validates and saves a Git repository outside automatic discovery roots", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    await addExplicitLocalRepository({ page: testPage, apiClient, backend, seedData });
  });
});

import { test } from "../../fixtures/test-base";
import {
  exerciseWatcherAutoStartFailureFlow,
  exerciseWatcherAutoStartFlow,
} from "./watcher-auto-start-flow";

test.describe("Sentry watcher auto-start defaults", () => {
  test("starts from the workspace profile before the task is opened", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await exerciseWatcherAutoStartFlow({
      page: testPage,
      api: apiClient,
      seedData,
      mobile: false,
    });
  });

  test("shows a real launch failure on the board and after reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await exerciseWatcherAutoStartFailureFlow({
      page: testPage,
      api: apiClient,
      seedData,
      mobile: false,
    });
  });
});

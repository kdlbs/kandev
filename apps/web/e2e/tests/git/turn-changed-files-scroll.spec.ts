import { test } from "../../fixtures/test-base";
import { exerciseDelayedTurnCardScroll } from "./turn-changed-files-scroll-helpers";

test("follows delayed changed-file content after completion and reload", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  await exerciseDelayedTurnCardScroll(testPage, apiClient, seedData, prCapture);
});

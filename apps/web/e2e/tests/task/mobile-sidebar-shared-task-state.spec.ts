import { test } from "../../fixtures/test-base";
import { exerciseSharedContextRecovery } from "./sidebar-shared-context-fixtures";
import {
  exerciseSharedPaging,
  exerciseSharedFirstResponse,
} from "./sidebar-shared-task-state-fixtures";

test("phone pages complete shared state without queries and bounds archived ownership", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await exerciseSharedPaging(testPage, apiClient, seedData, true);
});
test("phone accepts safe first rows during three live invalidations", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await exerciseSharedFirstResponse(testPage, apiClient, seedData, true);
});

test("phone rejects old workspace pages and recovers shared coverage after reconnect", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await exerciseSharedContextRecovery(testPage, apiClient, seedData, true);
});

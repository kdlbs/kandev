import { test } from "../../fixtures/test-base";
import { exerciseSharedSidebarState } from "./sidebar-shared-state-helpers";

test("desktop renders first-time sidebar views from shared homepage state while queries are held", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await exerciseSharedSidebarState(testPage, apiClient, seedData, false);
});

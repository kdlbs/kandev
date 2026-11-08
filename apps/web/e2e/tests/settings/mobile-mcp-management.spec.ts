import { checkMCPSessionAdmission } from "../../helpers/mcp-session-admission";
import { checkWorkspaceProfileDestination } from "../../helpers/mcp-workspace-profile";
import { test } from "../../fixtures/test-base";
import {
  checkMCPUsageNavigation,
  checkGlobalProfileWorkspaceGuard,
  checkMCPUsagePaging,
} from "../../helpers/mcp-management";

test("opens named MCP usage and its owner on a phone", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  for (const width of [320, 393, 767, 900]) {
    await testPage.setViewportSize({ width, height: 850 });
    await checkMCPUsageNavigation(testPage, apiClient, seedData, true);
  }
});

test("guards global profile workspace changes", async ({ testPage, apiClient, seedData }) => {
  await checkGlobalProfileWorkspaceGuard(testPage, apiClient, seedData, true);
});

test("keeps partial usage, stale reads and unavailable owners distinct", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await checkMCPUsagePaging(testPage, apiClient, seedData);
});

test("opens workspace-owned profile metadata without global inventory leakage", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await checkWorkspaceProfileDestination(testPage, apiClient, seedData, true);
});

test("retries exact-session reads without opening a fallback conversation", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await checkMCPSessionAdmission(testPage, apiClient, seedData, true);
});

import { test, expect } from "../../fixtures/test-base";
import { watchOpeningFlow } from "./watch-dialog-ownership-flow";
import { withWatchFixture } from "./watch-dialog-ownership-fixture";

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.20, AC-INTEGRATIONS-GITLAB-INTEGRATION-001.21
// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.22
for (const [kind, create] of [
  ["review", false],
  ["issue", true],
] as const) {
  test(`${kind} ${create ? "create" : "update"} old ACK preserves the newer desktop draft`, async ({
    testPage,
    apiClient,
    seedData,
  }, info) => {
    await withWatchFixture(apiClient, seedData, info, (f) =>
      watchOpeningFlow(testPage, apiClient, seedData, { fixture: f, mobile: false, kind, create }),
    );
  });
}
test("owned watch fixture cleans up after a deliberate primary failure", async ({
  testPage,
  apiClient,
  seedData,
}, info) => {
  // Acquire the real page fixture before the seed checkout guard baseline.
  expect(testPage.isClosed()).toBe(false);
  const marker = new Error("controlled watch fixture primary failure");
  let id = "";
  await expect(
    withWatchFixture(apiClient, seedData, info, async (f) => {
      id = f.workspaceId;
      throw marker;
    }),
  ).rejects.toBe(marker);
  expect(
    (await apiClient.listWorkspaces()).workspaces.some((workspace) => workspace.id === id),
  ).toBe(false);
});

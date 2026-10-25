import { test } from "../../fixtures/test-base";
import { watchOpeningFlow } from "./watch-dialog-ownership-flow";
import { withWatchFixture } from "./watch-dialog-ownership-fixture";

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.20, AC-INTEGRATIONS-GITLAB-INTEGRATION-001.21
// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.22
for (const [kind, create] of [
  ["review", false],
  ["issue", true],
] as const) {
  test(`${kind} ${create ? "create" : "update"} old ACK preserves the newer phone draft with native taps`, async ({
    testPage,
    apiClient,
    seedData,
  }, info) => {
    await withWatchFixture(apiClient, seedData, info, (f) =>
      watchOpeningFlow(testPage, apiClient, seedData, { fixture: f, mobile: true, kind, create }),
    );
  });
}

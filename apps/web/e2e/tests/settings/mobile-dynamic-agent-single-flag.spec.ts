import { expect, test } from "../../fixtures/test-base";
import {
  assertSingleDynamicAgentToggle,
  openDynamicAgentFlagCatalog,
  useDynamicCircuitFlagEnvironment,
} from "./dynamic-agent-single-flag-helpers";

test.describe("Dynamic agent single-flag settings contract on mobile", () => {
  test("keeps the off control and environment lock when the old environment value is true", async ({
    testPage,
    backend,
  }) => {
    test.setTimeout(60_000);
    const releaseEnvironment = await useDynamicCircuitFlagEnvironment(backend, false, true);
    try {
      const { settings, catalog } = await openDynamicAgentFlagCatalog(testPage);
      await assertSingleDynamicAgentToggle(testPage, settings, catalog, false);
      await expect
        .poll(async () =>
          testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
        )
        .toBe(true);

      const refreshed = await openDynamicAgentFlagCatalog(testPage, true);
      await assertSingleDynamicAgentToggle(testPage, refreshed.settings, refreshed.catalog, false);
    } finally {
      await releaseEnvironment();
    }
  });

  test("keeps the on control and environment lock when the old environment value is false", async ({
    testPage,
    backend,
  }) => {
    test.setTimeout(60_000);
    const releaseEnvironment = await useDynamicCircuitFlagEnvironment(backend, true, false);
    try {
      const { settings, catalog } = await openDynamicAgentFlagCatalog(testPage);
      await assertSingleDynamicAgentToggle(testPage, settings, catalog, true);
      await expect
        .poll(async () =>
          testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
        )
        .toBe(true);

      const refreshed = await openDynamicAgentFlagCatalog(testPage, true);
      await assertSingleDynamicAgentToggle(testPage, refreshed.settings, refreshed.catalog, true);
    } finally {
      await releaseEnvironment();
    }
  });
});

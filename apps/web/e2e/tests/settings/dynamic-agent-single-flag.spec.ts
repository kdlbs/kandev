import { test } from "../../fixtures/test-base";
import {
  assertSingleDynamicAgentToggle,
  openDynamicAgentFlagCatalog,
  useDynamicCircuitFlagEnvironment,
} from "./dynamic-agent-single-flag-helpers";

test.describe("Dynamic agent single-flag settings contract", () => {
  test("ignores a stale true backoff environment value when routing is off", async ({
    testPage,
    backend,
  }) => {
    test.setTimeout(60_000);
    const releaseEnvironment = await useDynamicCircuitFlagEnvironment(backend, false, true);
    try {
      const { settings, catalog } = await openDynamicAgentFlagCatalog(testPage);
      await assertSingleDynamicAgentToggle(testPage, settings, catalog, false);

      const refreshed = await openDynamicAgentFlagCatalog(testPage, true);
      await assertSingleDynamicAgentToggle(testPage, refreshed.settings, refreshed.catalog, false);
    } finally {
      await releaseEnvironment();
    }
  });

  test("ignores a stale false backoff environment value when routing is on", async ({
    testPage,
    backend,
  }) => {
    test.setTimeout(60_000);
    const releaseEnvironment = await useDynamicCircuitFlagEnvironment(backend, true, false);
    try {
      const { settings, catalog } = await openDynamicAgentFlagCatalog(testPage);
      await assertSingleDynamicAgentToggle(testPage, settings, catalog, true);

      const refreshed = await openDynamicAgentFlagCatalog(testPage, true);
      await assertSingleDynamicAgentToggle(testPage, refreshed.settings, refreshed.catalog, true);
    } finally {
      await releaseEnvironment();
    }
  });
});

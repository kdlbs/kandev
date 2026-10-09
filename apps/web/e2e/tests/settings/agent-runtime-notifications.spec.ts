import { test } from "../../fixtures/test-base";
import { managedFallbackAwareness } from "./agent-runtime-fallback-helpers";
import { backendRuntimeUpdateSummary } from "./agent-runtime-summary-backend-helpers";
import { runtimeAwareness } from "./agent-runtime-notifications-helpers";

test("backend startup and reconnect deliver one delayed runtime summary", async ({
  testPage,
  backend,
  apiClient,
}) => {
  test.setTimeout(120_000);
  await backendRuntimeUpdateSummary(testPage, backend, apiClient, false);
});

test("runtime notices, saved consent and native guidance work outside Settings", async ({
  testPage,
  prCapture,
}) => {
  await runtimeAwareness(testPage, false, prCapture);
});

test("narrow fine-pointer runtime flow retains phone controls", async ({ testPage }) => {
  await testPage.setViewportSize({ width: 390, height: 844 });
  await runtimeAwareness(testPage, true);
});

for (const width of [767, 768]) {
  test(`runtime settings controls respect the ${width}px phone boundary`, async ({ testPage }) => {
    await testPage.setViewportSize({ width, height: 900 });
    await runtimeAwareness(testPage, width < 768);
  });
}

test("native host keeps managed fallback version controls reachable", async ({
  testPage,
  prCapture,
}) => {
  await managedFallbackAwareness(testPage, false, prCapture);
});

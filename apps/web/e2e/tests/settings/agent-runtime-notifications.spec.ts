import { test } from "../../fixtures/test-base";
import { managedFallbackAwareness } from "./agent-runtime-fallback-helpers";
import { runtimeAwareness } from "./agent-runtime-notifications-helpers";

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

test("native host keeps managed fallback version controls reachable", async ({
  testPage,
  prCapture,
}) => {
  await managedFallbackAwareness(testPage, false, prCapture);
});

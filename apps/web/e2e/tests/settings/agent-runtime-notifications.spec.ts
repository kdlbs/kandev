import { test } from "../../fixtures/test-base";
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

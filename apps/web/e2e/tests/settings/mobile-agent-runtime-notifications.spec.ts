import { test } from "../../fixtures/test-base";
import { managedFallbackAwareness } from "./agent-runtime-fallback-helpers";
import { runtimeAwareness } from "./agent-runtime-notifications-helpers";

test("phone runtime notices lead to saved policy, native guidance and version drawer", async ({
  testPage,
  prCapture,
}) => {
  await runtimeAwareness(testPage, true, prCapture);
});

test("native host keeps managed fallback version controls reachable", async ({
  testPage,
  prCapture,
}) => {
  await managedFallbackAwareness(testPage, true, prCapture);
});

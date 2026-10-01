import { test } from "../../fixtures/test-base";
import { runtimeAwareness } from "./agent-runtime-notifications-helpers";

test("phone runtime notices lead to saved policy, native guidance and version drawer", async ({
  testPage,
  prCapture,
}) => {
  await runtimeAwareness(testPage, true, prCapture);
});

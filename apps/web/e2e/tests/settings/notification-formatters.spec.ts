import { test } from "../../fixtures/test-base";
import { notificationFormatterScenario } from "./notification-formatters-helpers";

test("notification formatter activation, synthetic preview, and default restoration", async ({
  testPage,
  apiClient,
}, info) => {
  await notificationFormatterScenario(testPage, apiClient, info, false);
});

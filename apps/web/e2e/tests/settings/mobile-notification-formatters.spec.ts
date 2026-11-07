import { test } from "../../fixtures/test-base";
import { notificationFormatterScenario } from "./notification-formatters-helpers";

test("phone notification formatter activation and default restoration", async ({
  testPage,
  apiClient,
}, info) => {
  await notificationFormatterScenario(testPage, apiClient, info, true);
});

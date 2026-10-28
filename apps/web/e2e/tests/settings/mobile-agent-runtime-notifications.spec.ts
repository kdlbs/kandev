import { test } from "../../fixtures/test-base";
import { managedFallbackAwareness } from "./agent-runtime-fallback-helpers";
import { backendRuntimeUpdateSummary } from "./agent-runtime-summary-backend-helpers";
import { localizedReleaseNotice, runtimeAwareness } from "./agent-runtime-notifications-helpers";

// @covers AC-PLATFORM-I18N-001.1
test("phone application release notices use the active language", async ({
  testPage,
  backend,
  prCapture,
}) => {
  await localizedReleaseNotice(testPage, backend.frontendUrl, prCapture);
});

test("backend runtime summary reconnect replay stays deduplicated on phone", async ({
  testPage,
  backend,
  apiClient,
}) => {
  test.setTimeout(120_000);
  await backendRuntimeUpdateSummary(testPage, backend, apiClient, true);
});

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

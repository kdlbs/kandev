import { test } from "../../fixtures/test-base";
import { linkSentryIssue } from "./sentry-short-id-link-helpers";

for (const issueKey of ["WEB-1A", "WEB-123"]) {
  test(`desktop links Sentry ${issueKey}`, async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }, info) => {
    await linkSentryIssue(testPage, apiClient, seedData, {
      info,
      capture: prCapture,
      mobile: false,
      issueKey,
    });
  });
}

import { test } from "../../fixtures/test-base";
import { linkSentryIssue } from "./sentry-short-id-link-helpers";

for (const { label, issueKey, input, captureAsset, relink } of [
  { label: "base32", issueKey: "WEB-1A", input: "WEB-1A", captureAsset: false, relink: false },
  {
    label: "short ID URL",
    issueKey: "WEB-1A",
    input: "https://sentry.io/organizations/my-org/issues/WEB-1A/",
    captureAsset: false,
    relink: false,
  },
  { label: "numeric", issueKey: "WEB-123", input: "WEB-123", captureAsset: false, relink: false },
  { label: "relink", issueKey: "WEB-1A", input: "WEB-1A", captureAsset: true, relink: true },
]) {
  test(`desktop links Sentry ${label}`, async ({
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
      input,
      captureAsset,
      relink,
    });
  });
}

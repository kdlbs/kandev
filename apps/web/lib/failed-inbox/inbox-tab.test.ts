import { describe, expect, it } from "vitest";
import { INBOX_TAB_QUERY_PARAM, buildInboxTabHref, resolveInboxTab } from "./inbox-tab";

// AC-UI-INBOX-FAILED-001.2: the `tab` query parameter's only recognised
// values are `needs-you` and `failed`. An absent, empty, repeated, or
// unrecognised value renders Needs you rather than an error.
describe("resolveInboxTab", () => {
  it("selects failed for exactly one tab=failed value", () => {
    expect(resolveInboxTab(new URLSearchParams("tab=failed"))).toBe("failed");
  });

  it("selects needs-you when the param is absent", () => {
    expect(resolveInboxTab(new URLSearchParams(""))).toBe("needs-you");
  });

  it("selects needs-you when the param is empty", () => {
    expect(resolveInboxTab(new URLSearchParams("tab="))).toBe("needs-you");
  });

  it("selects needs-you when the param is repeated, even if both are failed", () => {
    expect(resolveInboxTab(new URLSearchParams("tab=failed&tab=failed"))).toBe("needs-you");
  });

  it("selects needs-you for an unrecognised value", () => {
    expect(resolveInboxTab(new URLSearchParams("tab=something-else"))).toBe("needs-you");
  });

  it("selects needs-you for the explicit needs-you value", () => {
    expect(resolveInboxTab(new URLSearchParams("tab=needs-you"))).toBe("needs-you");
  });
});

describe("buildInboxTabHref", () => {
  it("carries the selected tab as the only query parameter", () => {
    expect(buildInboxTabHref("/needs-you-inbox", "failed")).toBe(
      `/needs-you-inbox?${INBOX_TAB_QUERY_PARAM}=failed`,
    );
  });

  it("preserves other existing query parameters", () => {
    expect(
      buildInboxTabHref("/needs-you-inbox?other=1", "failed", new URLSearchParams("other=1")),
    ).toBe(`/needs-you-inbox?other=1&${INBOX_TAB_QUERY_PARAM}=failed`);
  });
});

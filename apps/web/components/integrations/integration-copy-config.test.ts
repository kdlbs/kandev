import { describe, it, expect } from "vitest";
import { integrationFromPathname, integrationLabel } from "./integration-copy-config";

describe("integrationFromPathname", () => {
  it("resolves each supported integration slug", () => {
    expect(integrationFromPathname("/settings/integrations/slack")).toBe("slack");
    expect(integrationFromPathname("/settings/integrations/jira")).toBe("jira");
    expect(integrationFromPathname("/settings/integrations/linear")).toBe("linear");
    expect(integrationFromPathname("/settings/integrations/sentry")).toBe("sentry");
    expect(integrationFromPathname("/settings/integrations/github")).toBe("github");
  });

  it("ignores trailing path segments and query-like suffixes", () => {
    expect(integrationFromPathname("/settings/integrations/slack/watchers")).toBe("slack");
  });

  it("returns null for non-integration or unknown pages", () => {
    expect(integrationFromPathname("/settings/integrations")).toBeNull();
    expect(integrationFromPathname("/settings/agents")).toBeNull();
    expect(integrationFromPathname("/settings/integrations/unknown")).toBeNull();
  });
});

describe("integrationLabel", () => {
  it("uses the app's canonical spelling", () => {
    expect(integrationLabel("github")).toBe("GitHub");
    expect(integrationLabel("jira")).toBe("Jira");
    expect(integrationLabel("slack")).toBe("Slack");
    expect(integrationLabel("linear")).toBe("Linear");
    expect(integrationLabel("sentry")).toBe("Sentry");
  });
});

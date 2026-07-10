import { describe, expect, it } from "vitest";

import { integrationFromPathname } from "./integration-copy-config";

describe("integrationFromPathname", () => {
  it("recognizes global integration settings routes", () => {
    expect(integrationFromPathname("/settings/integrations/github")).toBe("github");
    expect(integrationFromPathname("/settings/integrations/linear")).toBe("linear");
  });

  it("recognizes workspace-scoped integration settings routes", () => {
    expect(integrationFromPathname("/settings/workspace/ws-1/integrations/github")).toBe("github");
    expect(integrationFromPathname("/settings/workspace/ws-1/integrations/linear")).toBe("linear");
  });

  it("ignores non-copyable integration routes", () => {
    expect(integrationFromPathname("/settings/workspace/ws-1/integrations/gitlab")).toBeNull();
    expect(integrationFromPathname("/settings/workspace/ws-1/integrations")).toBeNull();
  });
});

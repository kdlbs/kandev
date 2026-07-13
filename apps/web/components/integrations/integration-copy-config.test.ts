import { afterEach, describe, expect, it, vi } from "vitest";

import { copySentryInstances } from "@/lib/api/domains/sentry-api";
import { copyIntegrationConfig, integrationFromPathname } from "./integration-copy-config";

vi.mock("@/lib/api/domains/sentry-api", () => ({
  copySentryInstances: vi.fn(),
}));

afterEach(() => {
  vi.clearAllMocks();
});

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

describe("copyIntegrationConfig", () => {
  it("scopes Sentry copies to the source workspace and sends only the target", async () => {
    await copyIntegrationConfig("sentry", "ws-source", "ws-target");

    expect(copySentryInstances).toHaveBeenCalledWith("ws-target", { workspaceId: "ws-source" });
  });
});

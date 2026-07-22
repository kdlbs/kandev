import { afterEach, describe, expect, it, vi } from "vitest";

import { copySentryInstances } from "@/lib/api/domains/sentry-api";
import { copyAzureDevOpsConfig } from "@/lib/api/domains/azure-devops-api";
import { copyGitLabConfig } from "@/lib/api/domains/gitlab-api";
import { copyIntegrationConfig, integrationFromPathname } from "./integration-copy-config";

vi.mock("@/lib/api/domains/sentry-api", () => ({
  copySentryInstances: vi.fn(),
}));

vi.mock("@/lib/api/domains/azure-devops-api", () => ({
  copyAzureDevOpsConfig: vi.fn(),
}));

vi.mock("@/lib/api/domains/gitlab-api", () => ({
  copyGitLabConfig: vi.fn(),
}));

afterEach(() => {
  vi.clearAllMocks();
});

describe("integrationFromPathname", () => {
  it("recognizes global integration settings routes", () => {
    expect(integrationFromPathname("/settings/integrations/github")).toBe("github");
    expect(integrationFromPathname("/settings/integrations/linear")).toBe("linear");
    expect(integrationFromPathname("/settings/integrations/azure-devops")).toBe("azure-devops");
  });

  it("recognizes workspace-scoped integration settings routes", () => {
    expect(integrationFromPathname("/settings/workspace/ws-1/integrations/github")).toBe("github");
    expect(integrationFromPathname("/settings/workspace/ws-1/integrations/linear")).toBe("linear");
  });

  it("ignores non-copyable integration routes", () => {
    expect(integrationFromPathname("/settings/workspace/ws-1/integrations/gitlab")).toBe("gitlab");
    expect(integrationFromPathname("/settings/workspace/ws-1/integrations")).toBeNull();
  });

  it("copies the GitLab connection within the selected workspace boundary", async () => {
    await copyIntegrationConfig("gitlab", "ws-source", "ws-target");

    expect(copyGitLabConfig).toHaveBeenCalledWith("ws-target", { workspaceId: "ws-source" });
  });
});

describe("copyIntegrationConfig", () => {
  it("scopes Sentry copies to the source workspace and sends only the target", async () => {
    await copyIntegrationConfig("sentry", "ws-source", "ws-target");

    expect(copySentryInstances).toHaveBeenCalledWith("ws-target", { workspaceId: "ws-source" });
  });

  it("copies Azure DevOps credentials between workspaces", async () => {
    await copyIntegrationConfig("azure-devops", "ws-source", "ws-target");

    expect(copyAzureDevOpsConfig).toHaveBeenCalledWith("ws-source", "ws-target");
  });
});

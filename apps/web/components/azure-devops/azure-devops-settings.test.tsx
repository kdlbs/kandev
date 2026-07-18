import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AzureDevOpsConfig } from "@/lib/types/azure-devops";

const mocks = vi.hoisted(() => ({
  deleteConfig: vi.fn(),
  getConfig: vi.fn(),
  setConfig: vi.fn(),
  testConnection: vi.fn(),
  toast: vi.fn(),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));
vi.mock("@/hooks/domains/integrations/use-integration-availability", () => ({
  INTEGRATION_STATUS_REFRESH_MS: 100_000,
}));
vi.mock("@/hooks/domains/azure-devops/use-azure-devops-projects", () => ({
  useAzureDevOpsProjects: () => ({
    data: [{ id: "project-old", name: "Old project", url: "old" }],
    loading: false,
    error: null,
    refresh: vi.fn(),
  }),
}));
vi.mock("@/lib/api/domains/azure-devops-api", () => ({
  deleteAzureDevOpsConfig: mocks.deleteConfig,
  getAzureDevOpsConfig: mocks.getConfig,
  setAzureDevOpsConfig: mocks.setConfig,
  testAzureDevOpsConnection: mocks.testConnection,
}));

import { AzureDevOpsConnectionSection } from "./azure-devops-settings";

const config: AzureDevOpsConfig = {
  workspaceId: "workspace-a",
  organizationUrl: "https://dev.azure.com/old-org",
  defaultProjectId: "project-old",
  defaultProjectName: "Old project",
  authMethod: "pat",
  hasSecret: true,
  lastOk: true,
  createdAt: "2026-07-18T00:00:00Z",
  updatedAt: "2026-07-18T00:00:00Z",
};

beforeEach(() => {
  vi.clearAllMocks();
  mocks.getConfig.mockResolvedValue(config);
  mocks.setConfig.mockResolvedValue({
    ...config,
    organizationUrl: "https://dev.azure.com/new-org",
    defaultProjectId: undefined,
    defaultProjectName: undefined,
  });
});

afterEach(cleanup);

describe("AzureDevOpsConnectionSection", () => {
  it("omits a project selected for the previous organization", async () => {
    render(<AzureDevOpsConnectionSection workspaceId="workspace-a" />);
    const organization = await screen.findByTestId("azure-devops-organization");
    await waitFor(() =>
      expect((organization as HTMLInputElement).value).toBe("https://dev.azure.com/old-org"),
    );

    fireEvent.change(organization, { target: { value: "https://dev.azure.com/new-org" } });
    fireEvent.change(screen.getByTestId("azure-devops-pat"), { target: { value: "new-pat" } });
    fireEvent.click(screen.getByTestId("azure-devops-save-button"));

    await waitFor(() => expect(mocks.setConfig).toHaveBeenCalledTimes(1));
    expect(mocks.setConfig).toHaveBeenCalledWith("workspace-a", {
      organizationUrl: "https://dev.azure.com/new-org",
      defaultProjectId: undefined,
      defaultProjectName: undefined,
      authMethod: "pat",
      pat: "new-pat",
    });
  });
});

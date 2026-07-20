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
  it("links to the organization PAT page and explains the required read scopes", async () => {
    render(<AzureDevOpsConnectionSection workspaceId="workspace-a" />);

    const patHelp = await screen.findByTestId("azure-devops-pat-help");
    const createTokenLink = screen.getByRole("link", { name: "Create personal access token" });

    expect(createTokenLink.getAttribute("href")).toBe(
      "https://dev.azure.com/old-org/_usersSettings/tokens",
    );
    expect(patHelp.textContent).toContain("Custom defined");
    expect(patHelp.textContent).toContain("Work Items");
    expect(patHelp.textContent).toContain("Code");
    expect(patHelp.textContent).toContain("Read");
  });

  it("does not create a token link from a non-Azure organization URL", async () => {
    render(<AzureDevOpsConnectionSection workspaceId="workspace-a" />);

    const organization = await screen.findByTestId("azure-devops-organization");
    await waitFor(() =>
      expect((organization as HTMLInputElement).value).toBe("https://dev.azure.com/old-org"),
    );
    fireEvent.change(organization, { target: { value: "https://example.com/old-org" } });

    expect(screen.queryByRole("link", { name: "Create personal access token" })).toBeNull();
    expect(screen.getByTestId("azure-devops-pat-help").textContent).toContain(
      "Enter a valid organization URL",
    );
  });

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

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import type { AzureDevOpsConfig } from "@/lib/types/azure-devops";

const mocks = vi.hoisted(() => ({
  getConfig: vi.fn(),
  setConfig: vi.fn(),
  available: vi.fn(),
  signOut: vi.fn(),
  toast: vi.fn(),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));
vi.mock("@/hooks/domains/integrations/use-integration-availability", () => ({
  INTEGRATION_STATUS_REFRESH_MS: 100_000,
}));
vi.mock("@/hooks/domains/azure-devops/use-azure-devops-projects", () => ({
  useAzureDevOpsProjects: () => ({ data: [], loading: false, error: null, refresh: vi.fn() }),
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: true, isMobile: false }),
}));
vi.mock("@/lib/api/domains/azure-devops-api", () => ({
  deleteAzureDevOpsConfig: vi.fn(),
  getAzureDevOpsConfig: mocks.getConfig,
  setAzureDevOpsConfig: mocks.setConfig,
  testAzureDevOpsConnection: vi.fn(),
}));
vi.mock("@/lib/api/domains/azure-devops-oauth-api", () => ({
  getAzureDevOpsOAuthAvailability: mocks.available,
  signOutAzureDevOpsOAuth: mocks.signOut,
  startAzureDevOpsOAuth: vi.fn(),
  completeAzureDevOpsOAuth: vi.fn(),
}));

import { AzureDevOpsConnectionSection } from "./azure-devops-settings";

const PAT_CONFIG: AzureDevOpsConfig = {
  workspaceId: "ws-1",
  organizationUrl: "https://dev.azure.com/acme",
  defaultProjectId: "p-1",
  defaultProjectName: "Platform",
  authMethod: "pat",
  hasSecret: true,
  lastOk: true,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};
const OAUTH_CONFIG: AzureDevOpsConfig = {
  ...PAT_CONFIG,
  authMethod: "oauth",
  oauthAccount: "Ana Silva",
};

const AZURE_CLI_CLIENT_ID = "04b07795-8ddb-461a-bbee-02f9e1bf7b46";

function renderSection() {
  render(
    <SettingsSaveProvider>
      <AzureDevOpsConnectionSection workspaceId="ws-1" />
    </SettingsSaveProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.available.mockResolvedValue({
    available: true,
    defaultClientId: "11111111-1111-1111-1111-111111111111",
    defaultRedirectUrl: "http://localhost:38429/api/v1/azure-devops/oauth/callback",
  });
});

afterEach(() => cleanup());

describe("AzureDevOpsConnectionSection authentication method", () => {
  it("shows the PAT field and no Microsoft block for a PAT connection", async () => {
    mocks.getConfig.mockResolvedValue(PAT_CONFIG);
    renderSection();

    await screen.findByTestId("azure-devops-auth-method");
    expect(screen.getByTestId("azure-devops-pat")).toBeTruthy();
    expect(screen.queryByTestId("azure-devops-oauth")).toBeNull();
  });

  it("shows the signed-in Microsoft account and saves without a PAT", async () => {
    mocks.getConfig.mockResolvedValue(OAUTH_CONFIG);
    mocks.setConfig.mockResolvedValue(OAUTH_CONFIG);
    renderSection();

    expect((await screen.findByTestId("azure-devops-oauth-account")).textContent).toContain(
      "Ana Silva",
    );
    expect(screen.queryByTestId("azure-devops-pat")).toBeNull();

    const save = screen.getByTestId("azure-devops-save-button") as HTMLButtonElement;
    expect(save.disabled).toBe(false);
    fireEvent.click(save);

    await waitFor(() => expect(mocks.setConfig).toHaveBeenCalled());
    const [, request] = mocks.setConfig.mock.calls[0];
    expect(request).toMatchObject({
      authMethod: "oauth",
      organizationUrl: OAUTH_CONFIG.organizationUrl,
    });
    expect(request).not.toHaveProperty("pat");
  });

  it("restores the workspace client ID and requires one without a default", async () => {
    mocks.available.mockResolvedValue({
      available: false,
      defaultClientId: "",
      defaultRedirectUrl: "",
    });
    mocks.getConfig.mockResolvedValue({
      ...OAUTH_CONFIG,
      oauthClientId: AZURE_CLI_CLIENT_ID,
    });
    renderSection();

    const clientId = (await screen.findByTestId(
      "azure-devops-oauth-client-id",
    )) as HTMLInputElement;
    expect(clientId.value).toBe(AZURE_CLI_CLIENT_ID);
    expect(screen.queryByTestId("azure-devops-oauth-redirect-url")).toBeNull();
    expect(screen.queryByTestId("azure-devops-oauth-client-required")).toBeNull();

    fireEvent.change(clientId, { target: { value: "" } });
    expect(screen.getByTestId("azure-devops-oauth-client-required")).toBeTruthy();
  });

  it("restores the saved client ID after saving with the field cleared", async () => {
    const saved = { ...OAUTH_CONFIG, oauthClientId: AZURE_CLI_CLIENT_ID };
    mocks.getConfig.mockResolvedValue(saved);
    mocks.setConfig.mockResolvedValue(saved);
    renderSection();

    const clientId = (await screen.findByTestId(
      "azure-devops-oauth-client-id",
    )) as HTMLInputElement;
    fireEvent.change(clientId, { target: { value: "" } });
    fireEvent.click(screen.getByTestId("azure-devops-save-button"));

    await waitFor(() => expect(mocks.setConfig).toHaveBeenCalled());
    await waitFor(() =>
      expect((screen.getByTestId("azure-devops-oauth-client-id") as HTMLInputElement).value).toBe(
        AZURE_CLI_CLIENT_ID,
      ),
    );
  });

  it("signs out and offers Sign in with Microsoft again", async () => {
    mocks.getConfig.mockResolvedValue(OAUTH_CONFIG);
    mocks.signOut.mockResolvedValue({ ...OAUTH_CONFIG, hasSecret: false, oauthAccount: undefined });
    renderSection();

    fireEvent.click(await screen.findByTestId("azure-devops-oauth-sign-out"));

    await waitFor(() => expect(mocks.signOut).toHaveBeenCalledWith("ws-1"));
    await waitFor(() => expect(screen.queryByTestId("azure-devops-oauth-account")).toBeNull());
    expect(screen.queryByTestId("azure-devops-oauth-sign-out")).toBeNull();
    expect(mocks.toast).toHaveBeenCalledWith(expect.objectContaining({ variant: "success" }));
  });
});

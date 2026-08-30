import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import type { AzureDevOpsConfig } from "@/lib/types/azure-devops";

const mocks = vi.hoisted(() => ({
  deleteConfig: vi.fn(),
  getConfig: vi.fn(),
  setConfig: vi.fn(),
  testConnection: vi.fn(),
  toast: vi.fn(),
}));

let finePointer = true;

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
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: finePointer }),
}));
vi.mock("@/lib/api/domains/azure-devops-api", () => ({
  deleteAzureDevOpsConfig: mocks.deleteConfig,
  getAzureDevOpsConfig: mocks.getConfig,
  setAzureDevOpsConfig: mocks.setConfig,
  testAzureDevOpsConnection: mocks.testConnection,
}));
vi.mock("@/components/azure-devops/azure-devops-watch-settings", () => ({
  AzureDevOpsWatchSettings: () => (
    <>
      <section>
        <h3>Pull-request watches</h3>
      </section>
      <section>
        <h3>Work-item watches</h3>
      </section>
    </>
  ),
}));
vi.mock("@/components/azure-devops/azure-devops-quick-actions", () => ({
  AzureDevOpsQuickActionsSection: () => (
    <section>
      <h3>Quick actions</h3>
    </section>
  ),
}));
vi.mock("@/components/azure-devops/azure-devops-default-queries", () => ({
  AzureDevOpsDefaultQueriesSection: () => (
    <section>
      <h3>Default queries</h3>
    </section>
  ),
}));

import { AzureDevOpsConnectionSection, AzureDevOpsIntegrationPage } from "./azure-devops-settings";

const OLD_ORGANIZATION_URL = "https://dev.azure.com/old-org";
const WORKSPACE_ID = "workspace-a";
const DELETE_BUTTON_TEST_ID = "azure-devops-delete-button";
const CONFIRM_POPOVER_TEST_ID = "azure-devops-remove-confirm-popover";

const config: AzureDevOpsConfig = {
  workspaceId: WORKSPACE_ID,
  organizationUrl: OLD_ORGANIZATION_URL,
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
  finePointer = true;
  mocks.getConfig.mockResolvedValue(config);
  mocks.setConfig.mockResolvedValue({
    ...config,
    organizationUrl: "https://dev.azure.com/new-org",
    defaultProjectId: undefined,
    defaultProjectName: undefined,
  });
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("AzureDevOpsConnectionSection", () => {
  it("links to the organization PAT page and explains the required read scopes", async () => {
    render(
      <SettingsSaveProvider>
        <AzureDevOpsConnectionSection workspaceId={WORKSPACE_ID} />
      </SettingsSaveProvider>,
    );

    const patHelpButton = await screen.findByRole("button", {
      name: "How to create a personal access token",
    });
    expect(screen.queryByTestId("azure-devops-pat-help")).toBeNull();
    fireEvent.focus(patHelpButton);

    const patHelp = await screen.findByTestId("azure-devops-pat-help");
    const [createTokenLink] = within(patHelp).getAllByRole("link", {
      name: "Create personal access token",
    });

    expect(createTokenLink.getAttribute("href")).toBe(
      `${OLD_ORGANIZATION_URL}/_usersSettings/tokens`,
    );
    expect(patHelp.textContent).toContain("Custom defined");
    // The scope step is a `<Trans>` whose `<n>` indices address the JSX children
    // positionally, so a reflow of those children silently reassembles the
    // sentence into fragments. Assert the whole reconstructed clause, not just
    // that the two scope names appear somewhere.
    expect(patHelp.textContent).toContain("Under Work Items, check Read & write.");
    expect(patHelp.textContent).toContain("Under Code, check Read.");
    expect(patHelp.textContent).toContain("Leave all other scopes unchecked.");
  });

  it("does not create a token link from a non-Azure organization URL", async () => {
    render(
      <SettingsSaveProvider>
        <AzureDevOpsConnectionSection workspaceId={WORKSPACE_ID} />
      </SettingsSaveProvider>,
    );

    const organization = await screen.findByTestId("azure-devops-organization");
    await waitFor(() =>
      expect((organization as HTMLInputElement).value).toBe(OLD_ORGANIZATION_URL),
    );
    fireEvent.change(organization, { target: { value: "https://example.com/old-org" } });
    fireEvent.focus(screen.getByRole("button", { name: "How to create a personal access token" }));

    const patHelp = screen.getByTestId("azure-devops-pat-help");
    expect(
      within(patHelp).queryByRole("link", { name: "Create personal access token" }),
    ).toBeNull();
    expect(patHelp.textContent).toContain("Enter a valid organization URL");
  });

  it("removes trailing slashes before saving an organization URL", async () => {
    render(
      <SettingsSaveProvider>
        <AzureDevOpsConnectionSection workspaceId={WORKSPACE_ID} />
      </SettingsSaveProvider>,
    );
    const organization = await screen.findByTestId("azure-devops-organization");
    await waitFor(() =>
      expect((organization as HTMLInputElement).value).toBe(OLD_ORGANIZATION_URL),
    );

    fireEvent.change(organization, { target: { value: "https://dev.azure.com/old-org/" } });
    fireEvent.click(screen.getByTestId("azure-devops-save-button"));

    await waitFor(() => expect(mocks.setConfig).toHaveBeenCalledTimes(1));
    expect(mocks.setConfig).toHaveBeenCalledWith(WORKSPACE_ID, {
      organizationUrl: OLD_ORGANIZATION_URL,
      defaultProjectId: "project-old",
      defaultProjectName: "Old project",
      authMethod: "pat",
      pat: undefined,
    });
  });

  it("omits a project selected for the previous organization", async () => {
    render(
      <SettingsSaveProvider>
        <AzureDevOpsConnectionSection workspaceId={WORKSPACE_ID} />
      </SettingsSaveProvider>,
    );
    const organization = await screen.findByTestId("azure-devops-organization");
    await waitFor(() =>
      expect((organization as HTMLInputElement).value).toBe(OLD_ORGANIZATION_URL),
    );

    fireEvent.change(organization, { target: { value: "https://dev.azure.com/new-org" } });
    fireEvent.change(screen.getByTestId("azure-devops-pat"), { target: { value: "new-pat" } });
    fireEvent.click(screen.getByTestId("azure-devops-save-button"));

    await waitFor(() => expect(mocks.setConfig).toHaveBeenCalledTimes(1));
    expect(mocks.setConfig).toHaveBeenCalledWith(WORKSPACE_ID, {
      organizationUrl: "https://dev.azure.com/new-org",
      defaultProjectId: undefined,
      defaultProjectName: undefined,
      authMethod: "pat",
      pat: "new-pat",
    });
  });
});

describe("AzureDevOpsConnectionSection removal", () => {
  it("uses local fine-pointer confirmation and calls removal once after confirmation", async () => {
    const nativeConfirm = vi.fn(() => false);
    vi.stubGlobal("confirm", nativeConfirm);
    render(
      <SettingsSaveProvider>
        <AzureDevOpsConnectionSection workspaceId={WORKSPACE_ID} />
      </SettingsSaveProvider>,
    );

    const removeButton = await screen.findByTestId(DELETE_BUTTON_TEST_ID);
    fireEvent.click(removeButton);

    expect(nativeConfirm).not.toHaveBeenCalled();
    const popover = screen.getByTestId(CONFIRM_POPOVER_TEST_ID);
    expect(mocks.deleteConfig).not.toHaveBeenCalled();
    fireEvent.click(within(popover).getByRole("button", { name: "Cancel" }));
    expect(mocks.deleteConfig).not.toHaveBeenCalled();

    fireEvent.click(removeButton);
    fireEvent.click(
      within(screen.getByTestId(CONFIRM_POPOVER_TEST_ID)).getByTestId(
        "azure-devops-remove-confirm",
      ),
    );
    await waitFor(() => expect(mocks.deleteConfig).toHaveBeenCalledTimes(1));
    expect(mocks.deleteConfig).toHaveBeenCalledWith(WORKSPACE_ID);
  });

  it("morphs removal into touch-sized inline confirmation on coarse pointers", async () => {
    finePointer = false;
    render(
      <SettingsSaveProvider>
        <AzureDevOpsConnectionSection workspaceId={WORKSPACE_ID} />
      </SettingsSaveProvider>,
    );

    const removeButton = await screen.findByTestId(DELETE_BUTTON_TEST_ID);
    fireEvent.click(removeButton);
    const inline = screen.getByTestId("azure-devops-remove-inline-confirmation");
    expect(screen.queryByTestId(CONFIRM_POPOVER_TEST_ID)).toBeNull();
    expect(within(inline).getByTestId("azure-devops-remove-confirm").className).toContain("h-11");

    fireEvent.click(within(inline).getByRole("button", { name: "Cancel" }));
    expect(mocks.deleteConfig).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId(DELETE_BUTTON_TEST_ID));
    fireEvent.click(
      within(screen.getByTestId("azure-devops-remove-inline-confirmation")).getByTestId(
        "azure-devops-remove-confirm",
      ),
    );
    await waitFor(() => expect(mocks.deleteConfig).toHaveBeenCalledTimes(1));
  });

  it("clears confirmation when polling removes and later restores the configuration", async () => {
    vi.useFakeTimers();
    mocks.getConfig
      .mockResolvedValueOnce(config)
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce(config);
    render(
      <SettingsSaveProvider>
        <AzureDevOpsConnectionSection workspaceId={WORKSPACE_ID} />
      </SettingsSaveProvider>,
    );

    await act(async () => {
      await Promise.resolve();
    });
    fireEvent.click(screen.getByTestId(DELETE_BUTTON_TEST_ID));
    expect(screen.getByTestId(CONFIRM_POPOVER_TEST_ID)).toBeTruthy();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(100_000);
    });
    expect(screen.queryByTestId(DELETE_BUTTON_TEST_ID)).toBeNull();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(100_000);
    });
    expect(screen.getByTestId(DELETE_BUTTON_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId(CONFIRM_POPOVER_TEST_ID)).toBeNull();
    expect(mocks.deleteConfig).not.toHaveBeenCalled();
  });
});

describe("AzureDevOpsIntegrationPage", () => {
  it("matches GitHub's analogous section order and includes default queries", async () => {
    render(
      <StateProvider>
        <SettingsSaveProvider>
          <AzureDevOpsIntegrationPage workspaceId={WORKSPACE_ID} />
        </SettingsSaveProvider>
      </StateProvider>,
    );

    await screen.findByText("Azure DevOps integration");
    const sectionTitles = screen
      .getAllByRole("heading")
      .map((heading) => heading.textContent)
      .filter((title) =>
        [
          "Azure DevOps integration",
          "Pull-request watches",
          "Work-item watches",
          "Quick actions",
          "Default queries",
        ].includes(title ?? ""),
      );
    expect(sectionTitles).toEqual([
      "Azure DevOps integration",
      "Pull-request watches",
      "Work-item watches",
      "Quick actions",
      "Default queries",
    ]);
  });
});

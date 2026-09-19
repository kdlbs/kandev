import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { JiraConfig } from "@/lib/types/jira";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";

const mocks = vi.hoisted(() => ({
  deleteConfig: vi.fn(),
  getConfig: vi.fn(),
  setConfig: vi.fn(),
  testConnection: vi.fn(),
  toast: vi.fn(),
}));

let finePointer = true;
const DELETE_BUTTON_TEST_ID = "jira-delete-button";
const CONFIRM_POPOVER_TEST_ID = "jira-remove-confirm-popover";

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));
vi.mock("@/hooks/domains/integrations/use-integration-availability", () => ({
  INTEGRATION_STATUS_REFRESH_MS: 100_000,
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: finePointer }),
}));
vi.mock("@/components/jira/jira-enabled-control", () => ({
  JiraEnabledControl: () => null,
}));
vi.mock("@/lib/api/domains/jira-api", () => ({
  deleteJiraConfig: mocks.deleteConfig,
  getJiraConfig: mocks.getConfig,
  setJiraConfig: mocks.setConfig,
  testJiraConnection: mocks.testConnection,
}));

import { JiraConnectionSection } from "./jira-settings";

const config: JiraConfig = {
  workspaceId: "workspace-a",
  siteUrl: "https://acme.atlassian.net",
  email: "alice@example.com",
  authMethod: "api_token",
  instanceType: "cloud",
  defaultProjectKey: "PROJ",
  hasSecret: true,
  lastOk: true,
  createdAt: "2026-07-18T00:00:00Z",
  updatedAt: "2026-07-18T00:00:00Z",
};

function renderSection() {
  return render(
    <SettingsSaveProvider>
      <JiraConnectionSection workspaceId="workspace-a" />
    </SettingsSaveProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.unstubAllGlobals();
  finePointer = true;
  mocks.getConfig.mockResolvedValue(config);
  mocks.deleteConfig.mockResolvedValue(undefined);
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("JiraConnectionSection removal", () => {
  it("uses local fine-pointer confirmation and invokes deletion once", async () => {
    const nativeConfirm = vi.fn(() => false);
    vi.stubGlobal("confirm", nativeConfirm);
    renderSection();

    const removeButton = await screen.findByTestId(DELETE_BUTTON_TEST_ID);
    fireEvent.click(removeButton);

    expect(nativeConfirm).not.toHaveBeenCalled();
    const popover = screen.getByTestId(CONFIRM_POPOVER_TEST_ID);
    expect(mocks.deleteConfig).not.toHaveBeenCalled();
    fireEvent.click(within(popover).getByRole("button", { name: "Cancel" }));
    expect(mocks.deleteConfig).not.toHaveBeenCalled();

    fireEvent.click(removeButton);
    fireEvent.click(
      within(screen.getByTestId(CONFIRM_POPOVER_TEST_ID)).getByTestId("jira-remove-confirm"),
    );
    await waitFor(() => expect(mocks.deleteConfig).toHaveBeenCalledTimes(1));
    expect(mocks.deleteConfig).toHaveBeenCalledWith({ workspaceId: "workspace-a" });
  });

  it("uses touch-sized inline confirmation on coarse pointers", async () => {
    finePointer = false;
    renderSection();

    const removeButton = await screen.findByTestId(DELETE_BUTTON_TEST_ID);
    fireEvent.click(removeButton);
    const inline = screen.getByTestId("jira-remove-inline-confirmation");
    expect(screen.queryByTestId(CONFIRM_POPOVER_TEST_ID)).toBeNull();
    expect(within(inline).getByTestId("jira-remove-confirm").className).toContain("h-11");

    fireEvent.click(within(inline).getByRole("button", { name: "Cancel" }));
    expect(mocks.deleteConfig).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId(DELETE_BUTTON_TEST_ID));
    fireEvent.click(
      within(screen.getByTestId("jira-remove-inline-confirmation")).getByTestId(
        "jira-remove-confirm",
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
    renderSection();

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

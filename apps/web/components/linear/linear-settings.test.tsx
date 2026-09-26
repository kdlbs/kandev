import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { LinearConfig } from "@/lib/types/linear";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";

const REMOVE_CONFIRM_TEST_ID = "linear-remove-confirm";

const mocks = vi.hoisted(() => ({
  deleteConfig: vi.fn(),
  getConfig: vi.fn(),
  listTeams: vi.fn(),
  setConfig: vi.fn(),
  testConnection: vi.fn(),
  toast: vi.fn(),
}));

let finePointer = true;
let isMobile = false;
const DELETE_BUTTON_TEST_ID = "linear-delete-button";
const CONFIRM_POPOVER_TEST_ID = "linear-remove-confirm-popover";

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));
vi.mock("@/hooks/domains/integrations/use-integration-availability", () => ({
  INTEGRATION_STATUS_REFRESH_MS: 100_000,
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: finePointer, isMobile }),
}));
vi.mock("@/components/linear/linear-enabled-control", () => ({
  LinearEnabledControl: () => null,
}));
vi.mock("@/lib/api/domains/linear-api", () => ({
  deleteLinearConfig: mocks.deleteConfig,
  getLinearConfig: mocks.getConfig,
  listLinearTeams: mocks.listTeams,
  setLinearConfig: mocks.setConfig,
  testLinearConnection: mocks.testConnection,
}));

import { LinearConnectionSection } from "./linear-settings";

const config: LinearConfig = {
  workspaceId: "workspace-a",
  authMethod: "api_key",
  defaultTeamKey: "ENG",
  hasSecret: true,
  lastOk: true,
  createdAt: "2026-07-18T00:00:00Z",
  updatedAt: "2026-07-18T00:00:00Z",
};

function renderSection() {
  return render(
    <SettingsSaveProvider>
      <LinearConnectionSection workspaceId="workspace-a" />
    </SettingsSaveProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.unstubAllGlobals();
  finePointer = true;
  isMobile = false;
  mocks.getConfig.mockResolvedValue(config);
  mocks.listTeams.mockResolvedValue({ teams: [] });
  mocks.deleteConfig.mockResolvedValue(undefined);
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("LinearConnectionSection removal", () => {
  it("keeps the phone form controls behind a sheet and cancels locally", async () => {
    isMobile = true;
    finePointer = false;
    renderSection();
    const trigger = await screen.findByTestId(DELETE_BUTTON_TEST_ID);
    fireEvent.click(trigger);
    const sheet = screen.getByRole("dialog");
    expect(sheet.getAttribute("data-slot")).toBe("drawer-content");
    expect(trigger.isConnected).toBe(true);
    fireEvent.click(within(sheet).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(document.activeElement).toBe(trigger));
    expect(mocks.deleteConfig).not.toHaveBeenCalled();
    fireEvent.click(trigger);
    fireEvent.click(screen.getByTestId(REMOVE_CONFIRM_TEST_ID));
    await waitFor(() =>
      expect(mocks.deleteConfig).toHaveBeenCalledExactlyOnceWith({ workspaceId: "workspace-a" }),
    );
  });
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
      within(screen.getByTestId(CONFIRM_POPOVER_TEST_ID)).getByTestId(REMOVE_CONFIRM_TEST_ID),
    );
    await waitFor(() => expect(mocks.deleteConfig).toHaveBeenCalledTimes(1));
    expect(mocks.deleteConfig).toHaveBeenCalledWith({ workspaceId: "workspace-a" });
  });

  it("uses touch-sized inline confirmation on coarse pointers", async () => {
    finePointer = false;
    renderSection();

    const removeButton = await screen.findByTestId(DELETE_BUTTON_TEST_ID);
    fireEvent.click(removeButton);
    const inline = screen.getByTestId("linear-remove-inline-confirmation");
    expect(screen.queryByTestId(CONFIRM_POPOVER_TEST_ID)).toBeNull();
    expect(within(inline).getByTestId(REMOVE_CONFIRM_TEST_ID).className).toContain("h-11");

    fireEvent.click(within(inline).getByRole("button", { name: "Cancel" }));
    expect(mocks.deleteConfig).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId(DELETE_BUTTON_TEST_ID));
    fireEvent.click(
      within(screen.getByTestId("linear-remove-inline-confirmation")).getByTestId(
        REMOVE_CONFIRM_TEST_ID,
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

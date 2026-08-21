import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { LinearConfig } from "@/lib/types/linear";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";

const mocks = vi.hoisted(() => ({
  deleteConfig: vi.fn(),
  getConfig: vi.fn(),
  listTeams: vi.fn(),
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
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: finePointer }),
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
  mocks.getConfig.mockResolvedValue(config);
  mocks.listTeams.mockResolvedValue({ teams: [] });
  mocks.deleteConfig.mockResolvedValue(undefined);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("LinearConnectionSection removal", () => {
  it("uses local fine-pointer confirmation and invokes deletion once", async () => {
    const nativeConfirm = vi.fn(() => false);
    vi.stubGlobal("confirm", nativeConfirm);
    renderSection();

    const removeButton = await screen.findByTestId("linear-delete-button");
    fireEvent.click(removeButton);

    expect(nativeConfirm).not.toHaveBeenCalled();
    const popover = screen.getByTestId("linear-remove-confirm-popover");
    expect(mocks.deleteConfig).not.toHaveBeenCalled();
    fireEvent.click(within(popover).getByRole("button", { name: "Cancel" }));
    expect(mocks.deleteConfig).not.toHaveBeenCalled();

    fireEvent.click(removeButton);
    fireEvent.click(
      within(screen.getByTestId("linear-remove-confirm-popover")).getByTestId(
        "linear-remove-confirm",
      ),
    );
    await waitFor(() => expect(mocks.deleteConfig).toHaveBeenCalledTimes(1));
    expect(mocks.deleteConfig).toHaveBeenCalledWith({ workspaceId: "workspace-a" });
  });

  it("uses touch-sized inline confirmation on coarse pointers", async () => {
    finePointer = false;
    renderSection();

    const removeButton = await screen.findByTestId("linear-delete-button");
    fireEvent.click(removeButton);
    const inline = screen.getByTestId("linear-remove-inline-confirmation");
    expect(screen.queryByTestId("linear-remove-confirm-popover")).toBeNull();
    expect(within(inline).getByTestId("linear-remove-confirm").className).toContain("h-11");

    fireEvent.click(within(inline).getByRole("button", { name: "Cancel" }));
    expect(mocks.deleteConfig).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("linear-delete-button"));
    fireEvent.click(
      within(screen.getByTestId("linear-remove-inline-confirmation")).getByTestId(
        "linear-remove-confirm",
      ),
    );
    await waitFor(() => expect(mocks.deleteConfig).toHaveBeenCalledTimes(1));
  });
});

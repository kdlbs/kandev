import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { deleteSentryInstance, listSentryInstances } from "@/lib/api/domains/sentry-api";
import type { SentryConfig } from "@/lib/types/sentry";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";

const mocks = vi.hoisted(() => ({
  activeWorkspaceId: "ws-active",
  setEnabled: vi.fn(),
  toast: vi.fn(),
  workspaces: [
    { id: "ws-active", name: "Active" },
    { id: "ws-route", name: "Route" },
  ],
}));

let finePointer = true;
const DELETE_BUTTON_TEST_ID = "sentry-instance-delete-button";
const CONFIRM_POPOVER_TEST_ID = "sentry-remove-confirm-popover";

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));

vi.mock("@/hooks/domains/sentry/use-sentry-enabled", () => ({
  useSentryEnabled: () => ({ enabled: true, setEnabled: mocks.setEnabled }),
}));

vi.mock("@/hooks/domains/integrations/use-integration-availability", () => ({
  INTEGRATION_STATUS_REFRESH_MS: 1,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: finePointer }),
}));

vi.mock("@kandev/ui/switch", () => ({
  Switch: ({ onCheckedChange: _onCheckedChange, ...props }: Record<string, unknown>) => (
    <button {...props} />
  ),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: {
      workspaces: { activeId: string; items: { id: string; name: string }[] };
    }) => unknown,
  ) =>
    selector({
      workspaces: {
        activeId: mocks.activeWorkspaceId,
        items: mocks.workspaces,
      },
    }),
}));

vi.mock("./sentry-issue-watchers-section", () => ({
  SentryIssueWatchersSection: ({ workspaceId }: { workspaceId: string }) => (
    <div data-testid="sentry-watchers-workspace">{workspaceId}</div>
  ),
}));

vi.mock("@/lib/api/domains/sentry-api", () => ({
  createSentryInstance: vi.fn(),
  deleteSentryInstance: vi.fn(),
  listSentryInstances: vi.fn(),
  sentryErrorCode: vi.fn(),
  sentryInUseWatchCount: vi.fn(),
  testSentryConnection: vi.fn(),
  testSentryInstance: vi.fn(),
  updateSentryInstance: vi.fn(),
  SENTRY_ERROR_CODES: { nameTaken: "SENTRY_INSTANCE_NAME_TAKEN" },
}));

import { SentryConnectionSection, SentryIntegrationPage } from "./sentry-settings";

function SettingsHarness({ children }: { children: ReactNode }) {
  return (
    <SettingsSaveProvider>
      <TooltipProvider>{children}</TooltipProvider>
    </SettingsSaveProvider>
  );
}

const instance: SentryConfig = {
  id: "instance-1",
  workspaceId: "workspace-1",
  name: "Production",
  authMethod: "auth_token",
  url: "https://sentry.example.com",
  hasSecret: true,
  lastOk: true,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};

beforeEach(() => {
  vi.useFakeTimers();
  mocks.activeWorkspaceId = "ws-active";
  finePointer = true;
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});

describe("SentryConnectionSection", () => {
  it("restores the add path when a polling refresh removes the instance being edited", async () => {
    vi.mocked(listSentryInstances).mockResolvedValueOnce([instance]).mockResolvedValueOnce([]);

    render(
      <SettingsHarness>
        <SentryConnectionSection workspaceId="workspace-1" />
      </SettingsHarness>,
    );

    await act(async () => {
      await Promise.resolve();
    });
    expect(screen.getByTestId("sentry-instance-edit-button")).toBeTruthy();
    fireEvent.click(screen.getByTestId("sentry-instance-edit-button"));
    expect(screen.getByTestId("sentry-edit-form")).toBeTruthy();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });

    expect(screen.getByRole("button", { name: "Add instance" })).toBeTruthy();
  });
});

describe("SentryConnectionSection stale delete confirmation", () => {
  it("clears delete confirmation when polling removes and later restores the instance", async () => {
    vi.mocked(listSentryInstances)
      .mockResolvedValueOnce([instance])
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([instance]);

    render(
      <SettingsHarness>
        <SentryConnectionSection workspaceId="workspace-1" />
      </SettingsHarness>,
    );

    await act(async () => {
      await Promise.resolve();
    });
    fireEvent.click(screen.getByTestId(DELETE_BUTTON_TEST_ID));
    expect(screen.getByTestId(CONFIRM_POPOVER_TEST_ID)).toBeTruthy();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(screen.queryByTestId("sentry-instance-card")).toBeNull();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(screen.getByTestId("sentry-instance-card")).toBeTruthy();
    expect(screen.queryByTestId(CONFIRM_POPOVER_TEST_ID)).toBeNull();
    expect(deleteSentryInstance).not.toHaveBeenCalled();
  });

  it("clears delete confirmation before editing the same instance", async () => {
    vi.mocked(listSentryInstances).mockResolvedValue([instance]);

    render(
      <SettingsHarness>
        <SentryConnectionSection workspaceId="workspace-1" />
      </SettingsHarness>,
    );

    await act(async () => {
      await Promise.resolve();
    });
    fireEvent.click(screen.getByTestId(DELETE_BUTTON_TEST_ID));
    expect(screen.getByTestId(CONFIRM_POPOVER_TEST_ID)).toBeTruthy();

    fireEvent.click(screen.getByTestId("sentry-instance-edit-button"));
    const form = screen.getByTestId("sentry-edit-form");
    fireEvent.click(within(form).getByRole("button", { name: "Cancel" }));

    expect(screen.getByTestId("sentry-instance-card")).toBeTruthy();
    expect(screen.queryByTestId(CONFIRM_POPOVER_TEST_ID)).toBeNull();
    expect(deleteSentryInstance).not.toHaveBeenCalled();
  });
});

describe("SentryConnectionSection loading and removal", () => {
  it("keeps initial load failures visible but silences recurring poll failures", async () => {
    vi.mocked(listSentryInstances).mockRejectedValue(new Error("offline"));

    render(
      <SettingsHarness>
        <SentryConnectionSection workspaceId="workspace-1" />
      </SettingsHarness>,
    );

    await act(async () => {
      await Promise.resolve();
    });
    expect(mocks.toast).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });

    expect(mocks.toast).toHaveBeenCalledTimes(1);
  });

  it("uses local fine-pointer confirmation and deletes one instance after confirmation", async () => {
    const nativeConfirm = vi.fn(() => false);
    vi.stubGlobal("confirm", nativeConfirm);
    vi.mocked(listSentryInstances).mockResolvedValueOnce([instance]).mockResolvedValueOnce([]);
    vi.mocked(deleteSentryInstance).mockResolvedValue({ deleted: true });

    render(
      <SettingsHarness>
        <SentryConnectionSection workspaceId="workspace-1" />
      </SettingsHarness>,
    );
    await act(async () => {
      await Promise.resolve();
    });

    const deleteButton = screen.getByTestId(DELETE_BUTTON_TEST_ID);
    fireEvent.click(deleteButton);
    expect(nativeConfirm).not.toHaveBeenCalled();
    const popover = screen.getByTestId(CONFIRM_POPOVER_TEST_ID);
    expect(deleteSentryInstance).not.toHaveBeenCalled();
    fireEvent.click(within(popover).getByRole("button", { name: "Cancel" }));
    expect(deleteSentryInstance).not.toHaveBeenCalled();

    fireEvent.click(deleteButton);
    fireEvent.click(
      within(screen.getByTestId(CONFIRM_POPOVER_TEST_ID)).getByTestId("sentry-remove-confirm"),
    );
    await act(async () => {
      vi.runAllTicks();
      await Promise.resolve();
    });
    expect(deleteSentryInstance).toHaveBeenCalledTimes(1);
    expect(deleteSentryInstance).toHaveBeenCalledWith("workspace-1", "instance-1");
  });

  it("uses touch-sized inline confirmation on coarse pointers", async () => {
    finePointer = false;
    vi.mocked(listSentryInstances).mockResolvedValue([instance]);
    vi.mocked(deleteSentryInstance).mockResolvedValue({ deleted: true });

    render(
      <SettingsHarness>
        <SentryConnectionSection workspaceId="workspace-1" />
      </SettingsHarness>,
    );
    await act(async () => {
      await Promise.resolve();
    });

    const deleteButton = screen.getByTestId(DELETE_BUTTON_TEST_ID);
    fireEvent.click(deleteButton);
    const inline = screen.getByTestId("sentry-remove-inline-confirmation");
    expect(screen.queryByTestId(CONFIRM_POPOVER_TEST_ID)).toBeNull();
    expect(within(inline).getByTestId("sentry-remove-confirm").className).toContain("h-11");
    fireEvent.click(within(inline).getByRole("button", { name: "Cancel" }));
    expect(deleteSentryInstance).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId(DELETE_BUTTON_TEST_ID));
    fireEvent.click(
      within(screen.getByTestId("sentry-remove-inline-confirmation")).getByTestId(
        "sentry-remove-confirm",
      ),
    );
    await act(async () => {
      vi.runAllTicks();
      await Promise.resolve();
    });
    expect(deleteSentryInstance).toHaveBeenCalledTimes(1);
  });
});

describe("SentryIntegrationPage workspace scope", () => {
  it("passes the route workspace to watchers before the global active workspace", () => {
    vi.mocked(listSentryInstances).mockResolvedValue([]);

    render(
      <SettingsHarness>
        <SentryIntegrationPage workspaceId="ws-route" />
      </SettingsHarness>,
    );

    expect(screen.getByTestId("sentry-watchers-workspace").textContent).toBe("ws-route");
  });

  it("uses the global active workspace when no route workspace is supplied", () => {
    vi.mocked(listSentryInstances).mockResolvedValue([]);

    render(
      <SettingsHarness>
        <SentryIntegrationPage />
      </SettingsHarness>,
    );

    expect(screen.getByTestId("sentry-watchers-workspace").textContent).toBe("ws-active");
  });
});

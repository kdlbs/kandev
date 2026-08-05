import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { SettingsSaveContributor } from "./settings-save-provider";
import type { SleepInhibitionResponse } from "@/lib/types/system";
import { StateProvider } from "@/components/state-provider";

const fetchSettingsMock = vi.fn();
const updateSettingsMock = vi.fn();
let saveContributor: SettingsSaveContributor | null = null;
let currentRole: "admin" | "member" | undefined = "admin";
const TEST_SWITCH_ID = "sleep-inhibition-switch";

vi.mock("@/lib/api/domains/settings-api", () => ({
  fetchSleepInhibitionSettings: (...args: unknown[]) => fetchSettingsMock(...args),
  updateSleepInhibitionSettings: (...args: unknown[]) => updateSettingsMock(...args),
}));

vi.mock("./settings-save-provider", () => ({
  useSettingsSaveContributor: (contributor: SettingsSaveContributor) => {
    saveContributor = contributor;
  },
}));

import { SleepInhibitionSettings } from "./sleep-inhibition-settings";

function response(
  enabled = false,
  status: SleepInhibitionResponse["status"] = {
    platform: "linux",
    supported: true,
    active: false,
  },
): SleepInhibitionResponse {
  return { settings: { enabled }, status };
}

function renderSettings() {
  return render(
    <StateProvider
      initialState={{
        auth: {
          mode: "enabled",
          authenticated: true,
          user: currentRole
            ? {
                id: "user-1",
                email: "user@example.com",
                display_name: "Test User",
                role: currentRole,
                status: "active",
              }
            : null,
          ssoProviders: [],
        },
      }}
    >
      <SleepInhibitionSettings />
    </StateProvider>,
  );
}

beforeEach(() => {
  fetchSettingsMock.mockReset();
  updateSettingsMock.mockReset();
  fetchSettingsMock.mockResolvedValue(response());
  currentRole = "admin";
  saveContributor = null;
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.clearAllMocks();
});

describe("SleepInhibitionSettings", () => {
  it("explains the host sleep controls from an accessible info button", async () => {
    renderSettings();

    const infoButton = await screen.findByRole("button", {
      name: "How host sleep prevention works",
    });
    fireEvent.focus(infoButton);

    const tooltip = (await screen.findAllByRole("tooltip"))[0];
    expect(tooltip).toBeTruthy();
    expect(tooltip?.textContent).toContain("macOS runs");
    expect(tooltip?.textContent).toContain("/usr/bin/caffeinate -i -w <kandev-pid>");
    expect(tooltip?.textContent).toContain("Windows calls");
    expect(tooltip?.textContent).toContain("Linux asks systemd-logind");
  });

  it("stages an admin edit until the shared save contributor runs", async () => {
    updateSettingsMock.mockResolvedValueOnce(
      response(true, { platform: "linux", supported: true, active: true }),
    );
    renderSettings();

    const toggle = await screen.findByTestId(TEST_SWITCH_ID);
    fireEvent.click(toggle);
    expect(updateSettingsMock).not.toHaveBeenCalled();
    expect(saveContributor?.isDirty).toBe(true);
    if (!saveContributor) throw new Error("expected save contributor");

    await act(async () => saveContributor?.save(saveContributor.revision));

    expect(updateSettingsMock).toHaveBeenCalledWith({ enabled: true });
    await waitFor(() => expect(saveContributor?.isDirty).toBe(false));
    expect(screen.getByTestId("sleep-inhibition-status").textContent).toContain("Active");
  });

  it("keeps members read-only while preserving the configured value", async () => {
    currentRole = "member";
    fetchSettingsMock.mockResolvedValueOnce(
      response(true, {
        platform: "other",
        supported: false,
        active: false,
        issue: "unsupported_platform",
      }),
    );
    renderSettings();

    const toggle = await screen.findByTestId(TEST_SWITCH_ID);
    expect(toggle).toHaveProperty("disabled", true);
    expect(toggle.getAttribute("data-state")).toBe("checked");
    expect(screen.getByTestId("sleep-inhibition-status").textContent).toContain("Unavailable");
    expect(saveContributor?.isDirty).toBe(false);
  });

  it("reports failed saves without clearing the dirty draft", async () => {
    updateSettingsMock.mockRejectedValueOnce(new Error("offline"));
    renderSettings();
    fireEvent.click(await screen.findByTestId(TEST_SWITCH_ID));
    if (!saveContributor) throw new Error("expected save contributor");

    await act(async () => {
      await expect(saveContributor?.save(saveContributor.revision)).rejects.toThrow("offline");
    });

    expect(screen.getByText("Failed to save host sleep settings.")).toBeTruthy();
    expect(saveContributor?.isDirty).toBe(true);
  });

  it("refreshes runtime status without replacing an unsaved draft", async () => {
    vi.useFakeTimers();
    fetchSettingsMock.mockResolvedValueOnce(
      response(false, { platform: "linux", supported: true, active: false }),
    );
    renderSettings();

    await act(async () => {
      await Promise.resolve();
    });
    const toggle = screen.getByTestId(TEST_SWITCH_ID);
    fireEvent.click(toggle);
    expect(toggle.getAttribute("data-state")).toBe("checked");

    fetchSettingsMock.mockResolvedValueOnce(
      response(false, { platform: "linux", supported: true, active: true }),
    );
    await act(async () => {
      vi.advanceTimersByTime(15_000);
      await Promise.resolve();
    });

    expect(screen.getByTestId("sleep-inhibition-status").textContent).toContain("Active");
    expect(toggle.getAttribute("data-state")).toBe("checked");
    expect(saveContributor?.isDirty).toBe(true);
  });
});

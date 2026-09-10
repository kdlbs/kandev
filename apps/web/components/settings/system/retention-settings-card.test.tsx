import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { SettingsSaveContributor } from "@/components/settings/settings-save-provider";
import type { RetentionSettings, RetentionStatus } from "@/lib/types/system";
import { StateProvider } from "@/components/state-provider";

const fetchRetentionStatusMock = vi.fn();
const saveRetentionSettingsMock = vi.fn();
let saveContributor: SettingsSaveContributor | null = null;
let currentRole: "admin" | "member" | undefined = "admin";
const ENABLED_TOGGLE_TEST_ID = "retention-enabled";

vi.mock("@/lib/api/domains/system-api", () => ({
  fetchRetentionStatus: (...args: unknown[]) => fetchRetentionStatusMock(...args),
  saveRetentionSettings: (...args: unknown[]) => saveRetentionSettingsMock(...args),
}));

vi.mock("@/components/settings/settings-save-provider", () => ({
  useSettingsSaveContributor: (contributor: SettingsSaveContributor) => {
    saveContributor = contributor;
  },
}));

import { RetentionSettingsCard } from "./retention-settings-card";

function defaultSettings(overrides: Partial<RetentionSettings> = {}): RetentionSettings {
  return {
    enabled: true,
    sweep_interval_hours: 6,
    batch_limit: 5000,
    routine_runs: { window_days: 30, floor_per_owner: 50, warn_rows: 25000 },
    runs: { window_days: 30, floor_per_owner: 50, warn_rows: 25000 },
    run_events: { warn_rows: 250000 },
    ...overrides,
  };
}

function statusOf(overrides: Partial<RetentionStatus> = {}): RetentionStatus {
  return {
    settings: defaultSettings(),
    last_sweep: null,
    skip_count: 0,
    retained_counts: {
      office_routine_runs: { state: "not_computed", retained_count: 0, as_of: "" },
      runs: { state: "not_computed", retained_count: 0, as_of: "" },
      run_events: { state: "not_computed", retained_count: 0, as_of: "" },
    },
    ...overrides,
  };
}

function renderCard() {
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
      <RetentionSettingsCard />
    </StateProvider>,
  );
}

beforeEach(() => {
  fetchRetentionStatusMock.mockReset();
  saveRetentionSettingsMock.mockReset();
  fetchRetentionStatusMock.mockResolvedValue(statusOf());
  currentRole = "admin";
  saveContributor = null;
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("RetentionSettingsCard", () => {
  it("loads settings and renders the swept-table fields from the fetched status", async () => {
    renderCard();

    const windowDays = await screen.findByTestId("retention-routine-runs-window-days");
    expect(windowDays).toHaveProperty("value", "30");
    expect(screen.getByTestId("retention-runs-warn-rows")).toHaveProperty("value", "25000");
    expect(screen.getByTestId("retention-run-events-warn-rows")).toHaveProperty("value", "250000");
    expect(screen.getByTestId("retention-never-swept")).toBeTruthy();
  });

  it("stages an admin edit until the shared save contributor runs, then reloads", async () => {
    renderCard();
    await screen.findByTestId(ENABLED_TOGGLE_TEST_ID);

    const toggle = screen.getByTestId(ENABLED_TOGGLE_TEST_ID);
    fireEvent.click(toggle);
    expect(saveRetentionSettingsMock).not.toHaveBeenCalled();
    expect(saveContributor?.isDirty).toBe(true);
    if (!saveContributor) throw new Error("expected save contributor");

    saveRetentionSettingsMock.mockResolvedValueOnce(defaultSettings({ enabled: false }));
    fetchRetentionStatusMock.mockResolvedValueOnce(
      statusOf({ settings: defaultSettings({ enabled: false }) }),
    );

    await act(async () => saveContributor?.save(saveContributor.revision));

    expect(saveRetentionSettingsMock).toHaveBeenCalledWith(
      expect.objectContaining({ enabled: false }),
    );
    await waitFor(() => expect(saveContributor?.isDirty).toBe(false));
  });

  it("keeps members read-only while preserving the loaded values", async () => {
    currentRole = "member";
    renderCard();

    const windowDays = await screen.findByTestId("retention-routine-runs-window-days");
    expect(windowDays).toHaveProperty("disabled", true);
    expect(screen.getByTestId(ENABLED_TOGGLE_TEST_ID)).toHaveProperty("disabled", true);
    expect(screen.getByText("Only an admin can change retention settings.")).toBeTruthy();
    expect(saveContributor?.isDirty).toBe(false);
  });

  it("reports a failed save without clearing the dirty draft", async () => {
    renderCard();
    await screen.findByTestId(ENABLED_TOGGLE_TEST_ID);
    fireEvent.click(screen.getByTestId(ENABLED_TOGGLE_TEST_ID));
    if (!saveContributor) throw new Error("expected save contributor");

    saveRetentionSettingsMock.mockRejectedValueOnce(new Error("offline"));
    await act(async () => {
      await expect(saveContributor?.save(saveContributor.revision)).rejects.toThrow("offline");
    });

    expect(saveContributor?.isDirty).toBe(true);
    await screen.findByTestId("retention-save-error");
  });

  it("renders the last sweep outcome, backlog flag, and retained counts", async () => {
    fetchRetentionStatusMock.mockResolvedValue(
      statusOf({
        last_sweep: {
          started_at: "2026-09-01T00:00:00Z",
          finished_at: "2026-09-01T00:00:05Z",
          office_routine_runs: {
            deleted: 12,
            backlog: true,
            error: "",
            previewed: false,
            would_delete: 0,
          },
          runs: { deleted: 3, backlog: false, error: "", previewed: true, would_delete: 40 },
          run_events: { deleted: 100, backlog: false, error: "" },
          route_attempts: { deleted: 0, backlog: false, error: "" },
          run_skills: { deleted: 0, backlog: false, error: "" },
        },
        skip_count: 2,
        last_skip_at: "2026-09-01T00:10:00Z",
        retained_counts: {
          office_routine_runs: {
            state: "fresh",
            retained_count: 1200,
            as_of: "2026-09-01T00:00:00Z",
            top_routine_id: "routine-1",
            top_routine_share: 0.42,
          },
          runs: { state: "stale", retained_count: 800, as_of: "2026-08-31T00:00:00Z" },
          run_events: { state: "not_computed", retained_count: 0, as_of: "" },
        },
      }),
    );
    renderCard();

    await screen.findByTestId("retention-last-sweep");
    expect(screen.getByTestId("retention-backlog-office_routine_runs")).toBeTruthy();
    expect(screen.getByTestId("retention-retained-office_routine_runs").textContent).toContain(
      "1200",
    );
    expect(screen.getByTestId("retention-retained-office_routine_runs").textContent).toContain(
      "42%",
    );
    expect(screen.getByText(/Stale: last measurement failed/)).toBeTruthy();
    expect(screen.getByTestId("retention-skip-count").textContent).toContain("2");
  });
});

describe("RetentionSettingsCard unknown-status reporting", () => {
  it("renders each unrecognized status with its row count", async () => {
    fetchRetentionStatusMock.mockResolvedValue(
      statusOf({
        retained_counts: {
          office_routine_runs: {
            state: "fresh",
            retained_count: 5,
            as_of: "2026-09-01T00:00:00Z",
            unknown_statuses: [{ status: "quarantined", count: 2 }],
          },
          runs: { state: "not_computed", retained_count: 0, as_of: "" },
          run_events: { state: "not_computed", retained_count: 0, as_of: "" },
        },
      }),
    );
    renderCard();

    const row = await screen.findByTestId("retention-retained-office_routine_runs");
    expect(row.textContent).toContain("quarantined (2)");
  });
});

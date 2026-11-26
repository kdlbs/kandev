import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RuntimeStorageHealth } from "@/lib/types/http";
import { SessionDeliveryStorageNotice } from "./session-delivery-storage-notice";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, values?: Record<string, unknown>) =>
      values ? `${key} ${JSON.stringify(values)}` : key,
  }),
}));

vi.mock("@/lib/i18n/formats", () => ({
  formatDateTime: (date: Date) => date.toISOString(),
}));

const OBSERVED_AT = "2026-10-27T10:00:00Z";
const STALE_ATTRIBUTE = "data-stale";

const health: RuntimeStorageHealth = {
  state: "warning",
  location: "local_runtime",
  storage_kind: "journal",
  total_bytes: 1000,
  available_bytes: 100,
  observed_at: OBSERVED_AT,
  revision: 1,
  epoch: 5,
};

const NOTICE_TEST_ID = "session-delivery-storage-notice";
const DETAILS_TEST_ID = "session-delivery-storage-details";
const DETAILS_PANEL_TEST_ID = "session-delivery-storage-details-panel";
const SETTINGS_TEST_ID = "session-delivery-storage-settings";
const STORAGE_ADVISORY_USAGE = "task:storageAdvisoryUsage";
const STORAGE_ADVISORY_LOCAL_GUIDANCE = "task:storageAdvisoryGuidanceLocal";

afterEach(() => {
  cleanup();
  window.sessionStorage.clear();
  vi.restoreAllMocks();
});

describe("storage notice display", () => {
  it("renders one compact local warning row with settings and details", () => {
    render(<SessionDeliveryStorageNotice health={health} />);
    const row = screen.getByTestId(NOTICE_TEST_ID);
    expect(row.textContent).toContain(STORAGE_ADVISORY_USAGE);
    expect(row.textContent).toContain("task:storageAdvisoryFree");
    expect(screen.getByTestId(SETTINGS_TEST_ID).getAttribute("href")).toBe(
      "/settings/system/storage",
    );
    expect(screen.queryByTestId(DETAILS_PANEL_TEST_ID)).toBeNull();
    fireEvent.click(screen.getByTestId(DETAILS_TEST_ID));
    const panel = screen.getByTestId(DETAILS_PANEL_TEST_ID);
    expect(panel.textContent).toContain(STORAGE_ADVISORY_LOCAL_GUIDANCE);
    expect(panel.textContent).toContain("task:storageAdvisoryCapacityObserved");
    const detailsButton = screen.getByTestId(DETAILS_TEST_ID);
    expect(detailsButton.className).toContain("[@media(pointer:coarse)]:h-11");
    expect(detailsButton.className).toContain("cursor-pointer");
    expect(screen.getByTestId(SETTINGS_TEST_ID).className).toContain("cursor-pointer");
  });

  it("names the executor and omits local settings for remote storage", () => {
    render(
      <SessionDeliveryStorageNotice
        health={{ ...health, location: "executor_runtime", executor_label: "sprites" }}
      />,
    );
    const row = screen.getByTestId(NOTICE_TEST_ID);
    expect(row.textContent).toContain("task:storageAdvisoryRemotePrefix");
    expect(row.textContent).toContain(STORAGE_ADVISORY_USAGE);
    expect(screen.queryByTestId(SETTINGS_TEST_ID)).toBeNull();
  });

  it("keeps usage visible with a long executor label and full label in details", () => {
    const executor = "very-long-remote-executor-name-that-must-truncate";
    render(
      <SessionDeliveryStorageNotice
        health={{
          ...health,
          location: "executor_runtime",
          executor_label: executor,
          storage_kind: "workspace",
        }}
      />,
    );
    const row = screen.getByTestId(NOTICE_TEST_ID);
    expect(row.textContent).toContain(STORAGE_ADVISORY_USAGE);
    expect(row.querySelector(".flex-wrap")).toBeNull();
    fireEvent.click(screen.getByTestId(DETAILS_TEST_ID));
    expect(screen.getByTestId(DETAILS_PANEL_TEST_ID).textContent).toContain(executor);
  });
});

describe("capacity pressure guidance", () => {
  it("renders blocked delivery below 90% without claiming work continues", () => {
    render(
      <SessionDeliveryStorageNotice
        health={{
          ...health,
          state: "normal",
          total_bytes: 1000,
          available_bytes: 500,
          write_pressure: true,
          delivery_state: "delivery_failed",
          failure_storage: { reason: "disk_quota_exceeded", location: "local_runtime" },
        }}
      />,
    );
    const row = screen.getByTestId(NOTICE_TEST_ID);
    expect(row.getAttribute("data-pressure")).toBe("true");
    expect(row.textContent).toContain("task:storagePressureQuota");
    expect(row.textContent).not.toContain(STORAGE_ADVISORY_LOCAL_GUIDANCE);
    fireEvent.click(screen.getByTestId(DETAILS_TEST_ID));
    const panel = screen.getByTestId(DETAILS_PANEL_TEST_ID);
    expect(panel.textContent).toContain("task:storageRecoveryQuotaBodyLocal");
    expect(panel.textContent).toContain("task:storageAdvisoryPressureCause");
  });

  it("uses low-space guidance for a typed physical-capacity guard", () => {
    render(
      <SessionDeliveryStorageNotice
        health={{
          ...health,
          state: "normal",
          total_bytes: 1_000_000_000_000,
          available_bytes: 159_500_000_000,
          write_pressure: true,
          delivery_state: "delivery_failed",
          failure_storage: { reason: "disk_space_low", location: "local_runtime" },
        }}
      />,
    );
    const row = screen.getByTestId(NOTICE_TEST_ID);
    expect(row.textContent).toContain("task:storagePressureLowSpace");
    expect(row.textContent).toContain(STORAGE_ADVISORY_USAGE);
    expect(screen.getByTestId(SETTINGS_TEST_ID).getAttribute("href")).toBe(
      "/settings/system/storage",
    );
    fireEvent.click(screen.getByTestId(DETAILS_TEST_ID));
    expect(screen.getByTestId(DETAILS_PANEL_TEST_ID).textContent).toContain(
      "task:storageAdvisoryPressureGuidance",
    );
  });

  it("uses allocation guidance for a typed full-filesystem cause", () => {
    render(
      <SessionDeliveryStorageNotice
        health={{
          ...health,
          state: "normal",
          write_pressure: true,
          failure_storage: { reason: "filesystem_full", location: "local_runtime" },
        }}
      />,
    );
    fireEvent.click(screen.getByTestId(DETAILS_TEST_ID));
    expect(screen.getByTestId(DETAILS_PANEL_TEST_ID).textContent).toContain(
      "task:storageRecoveryFilesystemBodyLocal",
    );
  });

  it("uses neutral guidance when pressure has no typed cause", () => {
    render(
      <SessionDeliveryStorageNotice
        health={{
          ...health,
          state: "normal",
          total_bytes: 1000,
          available_bytes: 350,
          write_pressure: true,
          delivery_state: "delivery_failed",
        }}
      />,
    );
    const row = screen.getByTestId(NOTICE_TEST_ID);
    expect(row.textContent).toContain("task:storagePressureUnknown");
    expect(row.textContent).not.toContain(STORAGE_ADVISORY_USAGE);
    expect(screen.queryByTestId(SETTINGS_TEST_ID)).toBeNull();
    fireEvent.click(screen.getByTestId(DETAILS_TEST_ID));
    const panel = screen.getByTestId(DETAILS_PANEL_TEST_ID);
    expect(panel.textContent).toContain("task:storageAdvisoryPressureGuidanceUnknown");
    expect(panel.textContent).not.toContain(STORAGE_ADVISORY_LOCAL_GUIDANCE);
  });
});

describe("stream pressure guidance", () => {
  it("keeps stream-limit guidance and labels independent high usage in details", () => {
    render(
      <SessionDeliveryStorageNotice
        health={{
          ...health,
          state: "warning",
          total_bytes: 1000,
          available_bytes: 50,
          write_pressure: true,
          delivery_state: "delivery_failed",
          failure_storage: { reason: "stream_limit_reached", location: "local_runtime" },
        }}
      />,
    );
    const row = screen.getByTestId(NOTICE_TEST_ID);
    expect(row.textContent).toContain("task:storagePressureStreamLimit");
    expect(row.textContent).not.toContain(STORAGE_ADVISORY_USAGE);
    fireEvent.click(screen.getByTestId(DETAILS_TEST_ID));
    const panel = screen.getByTestId(DETAILS_PANEL_TEST_ID);
    expect(panel.textContent).toContain("task:storageAdvisoryPressureGuidanceLimit");
    expect(panel.textContent).toContain("task:storageAdvisoryHighUsage");
    expect(panel.textContent).toContain("task:storageAdvisoryFree");
    expect(panel.textContent).not.toContain(STORAGE_ADVISORY_LOCAL_GUIDANCE);
  });
});

describe("delivery pressure hysteresis", () => {
  it("keeps the disk advisory through the warning hysteresis during delivery pressure", () => {
    const failure = {
      reason: "stream_limit_reached",
      location: "local_runtime",
    } as const;
    const { rerender } = render(
      <SessionDeliveryStorageNotice
        health={{
          ...health,
          state: "warning",
          total_bytes: 1000,
          available_bytes: 50,
          write_pressure: true,
          delivery_state: "delivery_failed",
          failure_storage: failure,
        }}
      />,
    );
    fireEvent.click(screen.getByTestId(DETAILS_TEST_ID));
    const panel = screen.getByTestId(DETAILS_PANEL_TEST_ID);
    expect(panel.textContent).toContain('task:storageAdvisoryHighUsage {"usage":"95"}');

    rerender(
      <SessionDeliveryStorageNotice
        health={{
          ...health,
          state: "warning",
          total_bytes: 1000,
          available_bytes: 120,
          write_pressure: true,
          delivery_state: "delivery_failed",
          failure_storage: failure,
        }}
      />,
    );
    expect(panel.textContent).toContain('task:storageAdvisoryHighUsage {"usage":"88"}');
    expect(screen.getByTestId(SETTINGS_TEST_ID)).toBeTruthy();

    rerender(
      <SessionDeliveryStorageNotice
        health={{
          ...health,
          state: "normal",
          total_bytes: 1000,
          available_bytes: 200,
          write_pressure: true,
          delivery_state: "delivery_failed",
          failure_storage: failure,
        }}
      />,
    );
    expect(panel.textContent).not.toContain("task:storageAdvisoryHighUsage");
    expect(screen.queryByTestId(SETTINGS_TEST_ID)).toBeNull();
  });
});

describe("storage notice states", () => {
  it("does not recommend freeing disk space for an oversized output", () => {
    render(
      <SessionDeliveryStorageNotice
        health={{
          ...health,
          state: "normal",
          write_pressure: true,
          failure_storage: { reason: "event_too_large", location: "local_runtime" },
        }}
      />,
    );
    fireEvent.click(screen.getByTestId(DETAILS_TEST_ID));
    const panel = screen.getByTestId(DETAILS_PANEL_TEST_ID);
    expect(panel.textContent).toContain("task:storageAdvisoryPressureGuidanceOversized");
    expect(panel.textContent).not.toContain("task:storageAdvisoryPressureGuidance ");
  });

  it("labels stale receipt age and hides non-warning states", () => {
    render(<SessionDeliveryStorageNotice health={{ ...health, age_seconds: 120 }} />);
    expect(screen.getByTestId(NOTICE_TEST_ID).textContent).toContain(
      "task:storageAdvisoryLastReported",
    );
    cleanup();
    render(<SessionDeliveryStorageNotice health={{ ...health, state: "normal" }} />);
    expect(screen.queryByTestId(NOTICE_TEST_ID)).toBeNull();
    cleanup();
    render(<SessionDeliveryStorageNotice health={{ ...health, total_bytes: undefined }} />);
    expect(screen.queryByTestId(NOTICE_TEST_ID)).toBeNull();
  });
});

describe("pressure freshness", () => {
  it("keeps old pressure last reported while capacity stays fresh", () => {
    render(
      <SessionDeliveryStorageNotice
        health={{
          ...health,
          state: "normal",
          available_bytes: 500,
          age_seconds: 0,
          observed_at: new Date().toISOString(),
          write_pressure: true,
          pressure_observed_at: OBSERVED_AT,
          pressure_age_seconds: 120,
          failure_storage: { reason: "disk_quota_exceeded", location: "local_runtime" },
        }}
      />,
    );
    const row = screen.getByTestId(NOTICE_TEST_ID);
    expect(row.getAttribute(STALE_ATTRIBUTE)).toBe("true");
    expect(row.textContent).toContain("task:storageAdvisoryLastReported");
    fireEvent.click(screen.getByTestId(DETAILS_TEST_ID));
    const panel = screen.getByTestId(DETAILS_PANEL_TEST_ID);
    expect(panel.textContent).toContain("task:storageAdvisoryPressureObserved");
    expect(panel.textContent).toContain("task:storageAdvisoryCapacityObserved");
  });

  it("does not age fresh pressure from an older capacity sample", () => {
    render(
      <SessionDeliveryStorageNotice
        health={{
          ...health,
          state: "warning",
          age_seconds: 120,
          observed_at: OBSERVED_AT,
          write_pressure: true,
          pressure_observed_at: new Date().toISOString(),
          pressure_age_seconds: 0,
          failure_storage: { reason: "disk_quota_exceeded", location: "local_runtime" },
        }}
      />,
    );
    expect(screen.getByTestId(NOTICE_TEST_ID).getAttribute(STALE_ATTRIBUTE)).toBe("false");
    fireEvent.click(screen.getByTestId(DETAILS_TEST_ID));
    const panel = screen.getByTestId(DETAILS_PANEL_TEST_ID);
    expect(panel.textContent).toContain("task:storageAdvisoryPressureObserved");
    expect(panel.textContent).toContain("task:storageAdvisoryCapacityObserved");
  });

  it("keeps unknown pressure age last reported for an older backend", () => {
    render(
      <SessionDeliveryStorageNotice
        health={{
          ...health,
          state: "normal",
          write_pressure: true,
          age_seconds: 0,
          observed_at: new Date().toISOString(),
          failure_storage: { reason: "disk_quota_exceeded", location: "local_runtime" },
        }}
      />,
    );
    expect(screen.getByTestId(NOTICE_TEST_ID).getAttribute(STALE_ATTRIBUTE)).toBe("true");
    expect(screen.getByTestId(NOTICE_TEST_ID).textContent).toContain(
      "task:storageAdvisoryLastReported",
    );
  });
});

describe("pressure age clock", () => {
  it("does not double-count pressure age after a capacity refresh", () => {
    vi.useFakeTimers();
    let now = 0;
    const nowSpy = vi.spyOn(performance, "now").mockImplementation(() => now);
    try {
      const observedAt = "2026-10-27T10:00:00Z";
      const pressureObservedAt = new Date().toISOString();
      const base: RuntimeStorageHealth = {
        ...health,
        state: "normal",
        available_bytes: 500,
        age_seconds: 0,
        observed_at: observedAt,
        write_pressure: true,
        pressure_observed_at: pressureObservedAt,
        pressure_age_seconds: 0,
        failure_storage: { reason: "disk_quota_exceeded", location: "local_runtime" },
      };
      const { rerender } = render(<SessionDeliveryStorageNotice health={base} />);
      expect(screen.getByTestId(NOTICE_TEST_ID).getAttribute(STALE_ATTRIBUTE)).toBe("false");

      act(() => {
        now = 60_000;
        vi.advanceTimersByTime(60_000);
      });
      rerender(
        <SessionDeliveryStorageNotice
          health={{ ...base, revision: 2, pressure_age_seconds: 60 }}
        />,
      );
      expect(screen.getByTestId(NOTICE_TEST_ID).getAttribute(STALE_ATTRIBUTE)).toBe("false");

      act(() => {
        now = 91_000;
        vi.advanceTimersByTime(31_000);
      });
      expect(screen.getByTestId(NOTICE_TEST_ID).getAttribute(STALE_ATTRIBUTE)).toBe("true");

      rerender(
        <SessionDeliveryStorageNotice
          health={{ ...base, revision: 2, pressure_age_seconds: 91 }}
        />,
      );
      expect(screen.getByTestId(NOTICE_TEST_ID).getAttribute(STALE_ATTRIBUTE)).toBe("true");
    } finally {
      nowSpy.mockRestore();
      vi.useRealTimers();
    }
  });

  it("uses the cached browser receipt age after a remount", () => {
    const stale = {
      ...health,
      state: "normal" as const,
      available_bytes: 500,
      age_seconds: 0,
      observed_at: new Date().toISOString(),
      write_pressure: true,
      pressure_observed_at: new Date().toISOString(),
      pressure_age_seconds: 0,
      client_received_at_ms: Date.now() - 120_000,
      failure_storage: { reason: "disk_quota_exceeded", location: "local_runtime" as const },
    };
    render(<SessionDeliveryStorageNotice health={stale} />);
    expect(screen.getByTestId(NOTICE_TEST_ID).getAttribute(STALE_ATTRIBUTE)).toBe("true");
    cleanup();
    render(<SessionDeliveryStorageNotice health={stale} />);
    expect(screen.getByTestId(NOTICE_TEST_ID).getAttribute(STALE_ATTRIBUTE)).toBe("true");
  });
});

// @covers AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-004.1 through 004.5
describe("Storage notice dismissal", () => {
  it("hides details and remembers the task through updates, task switches, and remount", () => {
    const { rerender, unmount } = render(
      <SessionDeliveryStorageNotice taskId="task-a" health={health} />,
    );
    fireEvent.click(screen.getByTestId(DETAILS_TEST_ID));
    fireEvent.click(screen.getByRole("button", { name: "task:dismiss" }));
    expect(screen.queryByTestId(NOTICE_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(DETAILS_PANEL_TEST_ID)).toBeNull();
    const pressure = { ...health, revision: 2, write_pressure: true };
    rerender(<SessionDeliveryStorageNotice taskId="task-a" health={pressure} />);
    expect(screen.queryByTestId(NOTICE_TEST_ID)).toBeNull();
    rerender(<SessionDeliveryStorageNotice taskId="task-b" health={health} />);
    expect(screen.getByTestId(NOTICE_TEST_ID)).toBeTruthy();
    rerender(<SessionDeliveryStorageNotice taskId="task-a" health={health} />);
    expect(screen.queryByTestId(NOTICE_TEST_ID)).toBeNull();
    unmount();
    render(<SessionDeliveryStorageNotice taskId="task-a" health={health} />);
    expect(screen.queryByTestId(NOTICE_TEST_ID)).toBeNull();
  });

  it("dismisses locally when browser storage rejects reads and writes", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("denied");
    });
    render(<SessionDeliveryStorageNotice taskId="task-a" health={health} />);
    fireEvent.click(screen.getByRole("button", { name: "task:dismiss" }));
    expect(screen.queryByTestId(NOTICE_TEST_ID)).toBeNull();
  });

  it("keeps identity-free dismissal local to the mounted notice", () => {
    const { unmount } = render(<SessionDeliveryStorageNotice health={health} />);
    fireEvent.click(screen.getByRole("button", { name: "task:dismiss" }));
    expect(screen.queryByTestId(NOTICE_TEST_ID)).toBeNull();
    unmount();
    render(<SessionDeliveryStorageNotice health={health} />);
    expect(screen.getByTestId(NOTICE_TEST_ID)).toBeTruthy();
    expect(window.sessionStorage.length).toBe(0);
  });
});

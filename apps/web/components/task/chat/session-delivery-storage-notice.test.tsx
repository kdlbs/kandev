import { cleanup, fireEvent, render, screen } from "@testing-library/react";
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

const health: RuntimeStorageHealth = {
  state: "warning",
  location: "local_runtime",
  storage_kind: "journal",
  total_bytes: 1000,
  available_bytes: 100,
  observed_at: "2026-10-27T10:00:00Z",
  revision: 1,
  epoch: 5,
};

const NOTICE_TEST_ID = "session-delivery-storage-notice";
const DETAILS_TEST_ID = "session-delivery-storage-details";
const DETAILS_PANEL_TEST_ID = "session-delivery-storage-details-panel";
const SETTINGS_TEST_ID = "session-delivery-storage-settings";

afterEach(cleanup);

describe("SessionDeliveryStorageNotice", () => {
  it("renders one compact local warning row with settings and details", () => {
    render(<SessionDeliveryStorageNotice health={health} />);
    const row = screen.getByTestId(NOTICE_TEST_ID);
    expect(row.textContent).toContain("task:storageAdvisoryUsage");
    expect(row.textContent).toContain("task:storageAdvisoryFree");
    expect(screen.getByTestId(SETTINGS_TEST_ID).getAttribute("href")).toBe(
      "/settings/system/storage",
    );
    expect(screen.queryByTestId(DETAILS_PANEL_TEST_ID)).toBeNull();
    fireEvent.click(screen.getByTestId(DETAILS_TEST_ID));
    const panel = screen.getByTestId(DETAILS_PANEL_TEST_ID);
    expect(panel.textContent).toContain("task:storageAdvisoryGuidanceLocal");
    expect(panel.textContent).toContain("task:storageAdvisoryObserved");
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
    expect(row.textContent).toContain("task:storageAdvisoryUsage");
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
    expect(row.textContent).toContain("task:storageAdvisoryUsage");
    expect(row.querySelector(".flex-wrap")).toBeNull();
    fireEvent.click(screen.getByTestId(DETAILS_TEST_ID));
    expect(screen.getByTestId(DETAILS_PANEL_TEST_ID).textContent).toContain(executor);
  });

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
    expect(row.textContent).not.toContain("task:storageAdvisoryGuidanceLocal");
    fireEvent.click(screen.getByTestId(DETAILS_TEST_ID));
    const panel = screen.getByTestId(DETAILS_PANEL_TEST_ID);
    expect(panel.textContent).toContain("task:storageAdvisoryPressureGuidance");
    expect(panel.textContent).toContain("task:storageAdvisoryPressureCause");
  });

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

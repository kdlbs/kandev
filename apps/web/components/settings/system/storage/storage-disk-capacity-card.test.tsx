import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { StorageDiskCapacityResponse } from "@/lib/types/system";
import { StorageDiskCapacityCard } from "./storage-disk-capacity-card";

afterEach(cleanup);

const disk: StorageDiskCapacityResponse = {
  path: "/data",
  total_bytes: 100 * 1024 ** 3,
  used_bytes: 80 * 1024 ** 3,
  available_bytes: 20 * 1024 ** 3,
  used_percent: 80,
  available: true,
};

describe("StorageDiskCapacityCard", () => {
  it("renders the measured percentage, sizes, path, and accessible progress value", () => {
    render(<StorageDiskCapacityCard disk={disk} />);

    expect(screen.getByTestId("storage-disk-used-percent").textContent).toContain("80%");
    expect(screen.getByTestId("storage-disk-capacity-card").getAttribute("data-severity")).toBe(
      "warning",
    );
    expect(screen.getByTestId("storage-disk-capacity-path").textContent).toContain("/data");
    expect(screen.getByText("80 GB used")).toBeTruthy();
    expect(screen.getByText("20 GB available · 100 GB total")).toBeTruthy();
    expect(screen.getByRole("progressbar").getAttribute("aria-valuenow")).toBe("80");
  });

  it("uses critical styling at exactly 90% full", () => {
    render(
      <StorageDiskCapacityCard
        disk={{
          ...disk,
          used_percent: 90,
          used_bytes: 90 * 1024 ** 3,
          available_bytes: 10 * 1024 ** 3,
        }}
      />,
    );

    expect(screen.getByTestId("storage-disk-capacity-card").getAttribute("data-severity")).toBe(
      "critical",
    );
  });

  it("clamps invalid percentages before rendering the progress bar", () => {
    render(<StorageDiskCapacityCard disk={{ ...disk, used_percent: 120 }} />);

    expect(screen.getByRole("progressbar").getAttribute("aria-valuenow")).toBe("100");
    expect(screen.getByTestId("storage-disk-used-percent").textContent).toContain("100%");
  });

  it("renders an unavailable state without inventing capacity", () => {
    render(
      <StorageDiskCapacityCard disk={{ ...disk, available: false, warning: "unavailable" }} />,
    );

    expect(screen.getByTestId("storage-disk-unavailable")).toBeTruthy();
    expect(screen.queryByRole("progressbar")).toBeNull();
  });

  it("shows temporary filesystem pressure beside home capacity before analysis", () => {
    render(
      <StorageDiskCapacityCard
        disk={{
          ...disk,
          temporary_roots: [
            {
              requested_path: "/var/tmp",
              path: "/tmp",
              aliases: ["/var/tmp"],
              total_bytes: 100 * 1024 ** 3,
              used_bytes: 95 * 1024 ** 3,
              available_bytes: 5 * 1024 ** 3,
              used_percent: 95,
              available: true,
              observed_at: "2026-09-28T10:00:00Z",
              shared_with_home: false,
            },
          ],
        }}
      />,
    );

    const temporary = screen.getByTestId("storage-temporary-disk-capacity-0");
    expect(temporary.textContent).toContain("95%");
    expect(temporary.textContent).toContain("/tmp");
    expect(screen.getByTestId("storage-disk-view-temporary")).toBeTruthy();
  });
});

describe("temporary disk refresh state", () => {
  it("shows a failed temporary refresh while retaining stale capacity", () => {
    const props = {
      disk: {
        ...disk,
        temporary_roots_warning: "temporary storage paths unavailable",
        temporary_roots: [
          {
            requested_path: "/tmp",
            path: "/tmp",
            total_bytes: 100,
            used_bytes: 40,
            available_bytes: 60,
            used_percent: 40,
            available: true,
            observed_at: "2026-09-28T10:00:00.000Z",
            shared_with_home: false,
            stale: true,
          },
        ],
      },
    };
    const { rerender } = render(<StorageDiskCapacityCard {...props} />);

    expect(screen.getByTestId("storage-temporary-disk-refresh-failed")).toBeTruthy();
    expect(screen.getByTestId("storage-temporary-disk-stale").textContent).toContain(
      "latest refresh failed",
    );
    expect(screen.getByTestId("storage-temporary-disk-used-percent").textContent).toContain("40%");

    rerender(
      <StorageDiskCapacityCard
        {...props}
        disk={{ ...props.disk, temporary_roots_warning: undefined }}
        error="temporary capacity request failed"
      />,
    );
    expect(screen.getByTestId("storage-temporary-disk-refresh-failed")).toBeTruthy();
  });
});

describe("temporary disk unavailable state", () => {
  it("does not describe a never-measured capacity as a stale success", () => {
    render(
      <StorageDiskCapacityCard
        disk={{
          ...disk,
          temporary_roots: [
            {
              requested_path: "/tmp",
              path: "/tmp",
              total_bytes: 0,
              used_bytes: 0,
              available_bytes: 0,
              used_percent: 0,
              available: false,
              warning: "disk usage unavailable",
              observed_at: "2026-09-28T10:00:00.000Z",
              shared_with_home: null,
            },
          ],
        }}
      />,
    );

    expect(screen.getAllByTestId("storage-temporary-disk-unavailable").length).toBeGreaterThan(0);
    expect(screen.queryByTestId("storage-temporary-disk-stale")).toBeNull();
  });
});

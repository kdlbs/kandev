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

  it("uses critical styling when the filesystem is at least 90% full", () => {
    render(
      <StorageDiskCapacityCard
        disk={{
          ...disk,
          used_percent: 95,
          used_bytes: 95 * 1024 ** 3,
          available_bytes: 5 * 1024 ** 3,
        }}
      />,
    );

    expect(screen.getByTestId("storage-disk-capacity-card").getAttribute("data-severity")).toBe(
      "critical",
    );
  });

  it("renders an unavailable state without inventing capacity", () => {
    render(
      <StorageDiskCapacityCard disk={{ ...disk, available: false, warning: "unavailable" }} />,
    );

    expect(screen.getByTestId("storage-disk-unavailable")).toBeTruthy();
    expect(screen.queryByRole("progressbar")).toBeNull();
  });
});

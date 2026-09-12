import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { StorageRunHistory } from "./storage-run-history";

afterEach(cleanup);

describe("StorageRunHistory", () => {
  it("renders an independent loading state", () => {
    render(<StorageRunHistory runs={[]} loading />);

    expect(screen.getByTestId("storage-run-history-spinner")).toBeTruthy();
    expect(screen.getByText("Loading maintenance history…")).toBeTruthy();
    expect(screen.queryByText("No storage maintenance runs yet.")).toBeNull();
  });

  it("renders an isolated error state", () => {
    render(<StorageRunHistory runs={[]} error="history unavailable" />);

    expect(screen.getByTestId("storage-run-history-error").textContent).toContain(
      "history unavailable",
    );
    expect(screen.queryByTestId("storage-run-history-spinner")).toBeNull();
  });

  it("describes temporary artifact bytes as moved to quarantine", () => {
    render(
      <StorageRunHistory
        runs={[
          {
            id: "run-1",
            trigger: "manual",
            state: "succeeded",
            settings_snapshot: {} as never,
            result: {
              temporary_artifacts: {
                result: { quarantined_bytes: 2 * 1024 ** 3, reclaimed_bytes: 2 * 1024 ** 3 },
              },
            },
            message: "",
            started_at: "2026-07-23T12:00:00Z",
          },
        ]}
      />,
    );

    fireEvent.click(screen.getByTestId("storage-run-run-1").querySelector("button")!);
    expect(screen.getByTestId("storage-temporary-artifacts-result").textContent).toContain(
      "2 GB moved to quarantine.",
    );
    expect(screen.getByTestId("storage-temporary-artifacts-result").textContent).toContain(
      "Space is freed after permanent deletion.",
    );
  });
});

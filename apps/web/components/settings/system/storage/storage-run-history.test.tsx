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

  it("reports Go bytes removed, unknown remaining bytes, and partial cleanup", () => {
    render(
      <StorageRunHistory
        runs={[
          {
            id: "go-cache-run",
            trigger: "manual",
            state: "failed",
            settings_snapshot: {
              go_cache: { allow_cleanup_while_busy: true },
            } as never,
            result: {
              go_cache: {
                result: {
                  reclaimed_bytes: 6 * 1024 ** 3,
                  bytes_after: null,
                  partial: true,
                  errors: ["entry_delete_failed"],
                },
              },
              skipped_providers: { workspaces: { reason: "activity_busy" } },
            },
            message: "cleanup stopped early",
            started_at: "2026-10-02T12:00:00Z",
          },
        ]}
      />,
    );

    fireEvent.click(screen.getByTestId("storage-run-go-cache-run").querySelector("button")!);
    const result = screen.getByTestId("storage-go-cache-result");
    expect(result.textContent).toContain("6 GB removed");
    expect(result.textContent).toContain("The remaining cache size is unknown.");
    expect(result.textContent).toContain("Cleanup was partial.");
    expect(result.textContent).toContain(
      "Go cache cleanup during active tasks was enabled for this run.",
    );
    expect(result.textContent).toContain("Other cleanup was skipped because tasks are active.");
  });
});

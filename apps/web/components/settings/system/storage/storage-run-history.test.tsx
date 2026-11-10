import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { StorageMaintenanceRun } from "@/lib/types/system";
import { StorageRunHistory } from "./storage-run-history";

afterEach(cleanup);

const PARENT_CHANGE_ERROR = "go-cache parent changed after scan";
const FAILED = "failed" as const;
const RUN_STATUS = "storage-run-status";
const GO_CACHE_RESULT = "storage-go-cache-result";

function makeRun(overrides: Partial<StorageMaintenanceRun>): StorageMaintenanceRun {
  return {
    id: "run-1",
    trigger: "manual",
    state: FAILED,
    settings_snapshot: {} as never,
    result: {},
    message: "",
    started_at: "2026-11-03T05:54:07Z",
    ...overrides,
  };
}

function renderRuns(runs: StorageMaintenanceRun[]) {
  render(<StorageRunHistory runs={runs} />);
}

function openRun(id: string) {
  fireEvent.click(screen.getByTestId(`storage-run-${id}`).querySelector("button")!);
}

describe("StorageRunHistory states", () => {
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

  it("shows the empty history without section errors", () => {
    render(<StorageRunHistory runs={[]} />);
    expect(screen.getByText("No storage maintenance runs yet.")).toBeTruthy();
  });

  it("localizes status and trigger labels", () => {
    renderRuns([
      makeRun({
        id: "succeeded-run",
        trigger: "scheduled",
        state: "succeeded",
        result: { go_cache: { result: { reclaimed_bytes: 2 * 1024 ** 3, bytes_after: 0 } } },
      }),
    ]);

    expect(screen.getByTestId(RUN_STATUS).textContent).toBe("Succeeded");
    expect(screen.getByTestId("storage-run-trigger").textContent).toBe("Scheduled");
  });

  it("shows expected entry-limit progress as Incomplete", () => {
    renderRuns([
      makeRun({
        id: "progress-run",
        state: FAILED,
        result: {
          go_cache: {
            result: {
              reclaimed_bytes: 58.04 * 1024 ** 3,
              bytes_after: null,
              partial: true,
              reason: "cleanup_in_progress",
              errors: ["entry_limit_reached"],
            },
            error: "entry_limit_reached",
          },
        },
        message: "go_cache: entry_limit_reached",
      }),
    ]);

    expect(screen.getByTestId(RUN_STATUS).textContent).toBe("Incomplete");
    openRun("progress-run");
    expect(screen.getByTestId(GO_CACHE_RESULT).textContent).toContain(
      "The entry limit was reached. Cleanup can continue on a later run.",
    );
  });

  it("shows an unknown failure with a generic localized explanation", () => {
    renderRuns([
      makeRun({
        id: "unknown-run",
        state: FAILED,
        result: { go_cache: { result: { reclaimed_bytes: 0, errors: ["future_code"] } } },
        message: "go_cache: unexpected failure",
      }),
    ]);

    expect(screen.getByTestId(RUN_STATUS).textContent).toBe("Failed");
    openRun("unknown-run");
    expect(screen.getByTestId(GO_CACHE_RESULT).textContent).toContain(
      "Cleanup hit an unexpected error. View technical details.",
    );
  });

  it("explains a non-Go failure without a Go result", () => {
    renderRuns([
      makeRun({
        id: "non-go-run",
        state: FAILED,
        result: { quarantine: { result: { failed: 1 }, error: "purge failed" } },
        message: "quarantine: purge failed",
      }),
    ]);

    expect(screen.getByTestId(RUN_STATUS).textContent).toBe("Failed");
    openRun("non-go-run");
    expect(screen.getByTestId("storage-run-generic-explanation").textContent).toBe(
      "Other cleanup did not complete. View technical details.",
    );
    expect(screen.queryByTestId("storage-go-cache-result")).toBeNull();
  });
});

describe("StorageRunHistory partial explanations", () => {
  it("keeps a generic partial warning when a Go result has no issue codes", () => {
    renderRuns([
      makeRun({
        id: "partial-only",
        state: FAILED,
        result: {
          go_cache: {
            result: { reclaimed_bytes: 1024, bytes_after: null, partial: true },
          },
        },
      }),
    ]);

    expect(screen.getByTestId(RUN_STATUS).textContent).toBe("Partial");
    openRun("partial-only");
    expect(screen.getByTestId("storage-go-cache-partial-explanation").textContent).toBe(
      "Cleanup was partial.",
    );
  });

  it("shows a raw-only Go failure explanation", () => {
    renderRuns([
      makeRun({
        id: "raw-only",
        state: FAILED,
        result: {
          go_cache: {
            error: "permission denied",
            result: { reclaimed_bytes: 1024, bytes_after: null, partial: true },
          },
        },
      }),
    ]);

    openRun("raw-only");
    expect(screen.getByTestId("storage-go-cache-explanation").textContent).toBe(
      "Some cleanup did not finish.",
    );
  });
  it("shows a non-Go failure explanation on a failed run with Go categories", () => {
    renderRuns([
      makeRun({
        id: "mixed-failure",
        state: FAILED,
        result: {
          go_cache: {
            result: {
              reclaimed_bytes: 0,
              bytes_after: null,
              reason: "cleanup_in_progress",
              errors: ["entry_limit_reached"],
            },
            error: "entry_limit_reached",
          },
          workspaces: { result: {}, error: "permission denied" },
        },
        message: "go_cache: entry_limit_reached\nworkspaces: permission denied",
      }),
    ]);

    expect(screen.getByTestId(RUN_STATUS).textContent).toBe("Failed");
    openRun("mixed-failure");
    expect(screen.getByTestId("storage-run-generic-explanation").textContent).toBe(
      "Other cleanup did not complete. View technical details.",
    );
  });

  it("shows a generic explanation beside a known budget explanation", () => {
    renderRuns([
      makeRun({
        id: "budget-unknown",
        state: FAILED,
        result: {
          go_cache: {
            result: {
              reclaimed_bytes: 0,
              bytes_after: null,
              reason: "cleanup_in_progress",
              errors: ["entry_limit_reached"],
              partial: true,
            },
            error: "entry_limit_reached, permission denied",
          },
        },
        message: "go_cache: entry_limit_reached, permission denied",
      }),
    ]);

    expect(screen.getByTestId(RUN_STATUS).textContent).toBe("Failed");
    openRun("budget-unknown");
    expect(
      screen.getAllByTestId("storage-go-cache-explanation").map((element) => element.textContent),
    ).toEqual([
      "The entry limit was reached. Cleanup can continue on a later run.",
      "Some cleanup did not finish.",
    ]);
  });
});

describe("StorageRunHistory result details", () => {
  it("describes temporary artifact bytes as moved to quarantine", () => {
    renderRuns([
      makeRun({
        id: "run-1",
        state: "succeeded",
        result: {
          temporary_artifacts: {
            result: { quarantined_bytes: 2 * 1024 ** 3, reclaimed_bytes: 2 * 1024 ** 3 },
          },
        },
      }),
    ]);

    openRun("run-1");
    expect(screen.getByTestId("storage-temporary-artifacts-result").textContent).toContain(
      "2 GB moved to quarantine.",
    );
    expect(screen.getByTestId("storage-temporary-artifacts-result").textContent).toContain(
      "Space is freed after permanent deletion.",
    );
  });

  it("reports Go facts, one explanation, and keeps diagnostics initially closed", () => {
    renderRuns([
      makeRun({
        id: "go-cache-run",
        state: FAILED,
        settings_snapshot: { go_cache: { allow_cleanup_while_busy: true } } as never,
        result: {
          go_cache: {
            result: {
              reclaimed_bytes: 6 * 1024 ** 3,
              bytes_after: null,
              partial: true,
              errors: ["entry_delete_failed", "cache_parent_changed", "entry_delete_failed"],
            },
            error: `${PARENT_CHANGE_ERROR}\nentry_delete_failed`,
          },
          skipped_providers: { workspaces: { reason: "activity_busy" } },
        },
        message: `${PARENT_CHANGE_ERROR}\nentry_delete_failed`,
      }),
    ]);

    expect(screen.getByTestId(RUN_STATUS).textContent).toBe("Partial");
    openRun("go-cache-run");
    const result = screen.getByTestId(GO_CACHE_RESULT);
    expect(result.textContent).toContain("Removed: 6 GB");
    expect(result.textContent).toContain("Remaining: unknown");
    expect(result.textContent).toContain("Some cache entries changed during cleanup.");
    expect(screen.getAllByTestId("storage-go-cache-explanation")).toHaveLength(1);
    expect(result.textContent).toContain(
      "Go cache cleanup during active tasks was enabled for this run.",
    );
    expect(result.textContent).toContain("Other cleanup was skipped because tasks are active.");
    const details = screen.getByTestId("storage-run-technical-details") as HTMLDetailsElement;
    const trigger = screen.getByTestId("storage-run-technical-trigger");
    expect(details.tagName).toBe("DETAILS");
    expect(details.open).toBe(false);
    expect(trigger.tagName).toBe("SUMMARY");
    expect(screen.getByTestId("storage-run-technical-json").textContent).toContain(
      "entry_delete_failed",
    );

    fireEvent.click(trigger);
    expect(details.open).toBe(true);
    expect(screen.getByTestId("storage-run-technical-message").textContent).toContain(
      PARENT_CHANGE_ERROR,
    );

    fireEvent.click(trigger);
    expect(details.open).toBe(false);
  });
});

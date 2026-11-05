import { describe, expect, it } from "vitest";
import type { StorageMaintenanceRun } from "@/lib/types/system";
import { storageGoCacheResultSummary, storageRunSummary } from "./storage-run-summary";

const GIB = 1024 ** 3;
const PARENT_CHANGE_ERROR = "go-cache parent changed after scan";
const BUDGET_MESSAGE = "go_cache: entry_limit_reached";

function makeRun(overrides: Partial<StorageMaintenanceRun>): StorageMaintenanceRun {
  return {
    id: "run-1",
    trigger: "scheduled",
    state: "failed",
    settings_snapshot: { go_cache: { allow_cleanup_while_busy: false } } as never,
    result: {},
    message: "",
    started_at: "2026-11-03T05:54:07Z",
    ...overrides,
  };
}

function incidentRun(): StorageMaintenanceRun {
  return makeRun({
    id: "incident",
    state: "failed",
    settings_snapshot: { go_cache: { allow_cleanup_while_busy: true } } as never,
    result: {
      go_cache: {
        result: {
          reclaimed_bytes: 10.75 * GIB,
          bytes_after: 53.69 * GIB,
          partial: true,
          errors: ["entry_delete_failed", "cache_parent_changed", "entry_delete_failed"],
        },
        error: `${PARENT_CHANGE_ERROR}\nentry_delete_failed\n${PARENT_CHANGE_ERROR}`,
      },
      skipped_providers: {
        workspaces: { reason: "activity_busy" },
        quarantine: { reason: "activity_busy" },
      },
    },
    message: `go_cache: ${PARENT_CHANGE_ERROR}\nentry_delete_failed`,
  });
}

function progressRun(overrides: Partial<StorageMaintenanceRun> = {}): StorageMaintenanceRun {
  return makeRun({
    state: "failed",
    result: {
      go_cache: {
        result: {
          reclaimed_bytes: 58 * GIB,
          bytes_after: null,
          partial: true,
          reason: "cleanup_in_progress",
          errors: ["entry_limit_reached"],
        },
        error: "entry_limit_reached",
      },
    },
    message: BUDGET_MESSAGE,
    ...overrides,
  });
}

describe("storageRunSummary failures", () => {
  it("classifies the incident as partial with one distinct explanation", () => {
    const summary = storageRunSummary(incidentRun());

    expect(summary.status).toBe("partial");
    expect(summary.goCache?.reclaimedBytes).toBe(10.75 * GIB);
    expect(summary.goCache?.bytesAfter).toBe(53.69 * GIB);
    expect(summary.goCache?.issueCategories).toEqual(["entries_changed"]);
    expect(summary.goCache?.hasUnknownIssue).toBe(false);
    expect(summary.hasProviderError).toBe(true);
    expect(summary.hasNonGoProviderError).toBe(false);
    expect(summary.otherProvidersSkipped).toBe(true);
    expect(summary.busyPolicyEnabled).toBe(true);
  });

  it("shows expected entry-limit progress as incomplete", () => {
    const summary = storageRunSummary(progressRun());

    expect(summary.status).toBe("incomplete");
    expect(summary.goCache?.bytesAfter).toBeNull();
    expect(summary.goCache?.issueCategories).toEqual(["entry_limit"]);
    expect(summary.hasProviderError).toBe(true);
  });

  it("requires incomplete_scan progress when discovery stopped early", () => {
    const summary = storageRunSummary(
      progressRun({
        result: {
          go_cache: {
            result: {
              reclaimed_bytes: 0,
              bytes_after: null,
              reason: "incomplete_scan",
              errors: ["entry_limit_reached"],
            },
            error: "entry_limit_reached",
          },
        },
      }),
    );

    expect(summary.status).toBe("incomplete");
  });

  it("keeps a genuine provider failure from classifying as progress", () => {
    const summary = storageRunSummary(
      progressRun({
        result: {
          go_cache: {
            result: {
              reclaimed_bytes: 2 * GIB,
              bytes_after: null,
              reason: "cleanup_in_progress",
              errors: ["entry_limit_reached"],
            },
            error: "entry_limit_reached",
          },
          temporary_artifacts: {
            result: { reclaimed_bytes: 0, failed: 1, failed_bytes: 10 },
            error: "temporary artifact quarantine move failed",
          },
        },
        message: `${BUDGET_MESSAGE}\ntemporary_artifacts: move failed`,
      }),
    );

    expect(summary.status).toBe("partial");
    expect(summary.hasNonGoProviderError).toBe(true);
  });

  it("shows failed when a genuine error removed nothing", () => {
    const summary = storageRunSummary(
      makeRun({
        state: "failed",
        result: {
          quarantine: {
            result: { deleted_bytes: 0, failed: 2 },
            error: "purge failed",
          },
        },
        message: "quarantine: purge failed",
      }),
    );

    expect(summary.status).toBe("failed");
    expect(summary.hasNonGoProviderError).toBe(true);
    expect(summary.goCache).toBeNull();
  });
});

describe("storageRunSummary error evidence", () => {
  it("keeps malformed and unknown evidence visible as failures", () => {
    const summary = storageRunSummary(
      makeRun({
        state: "failed",
        result: {
          go_cache: {
            result: { reclaimed_bytes: 0, errors: ["mystery_code"] },
            error: { nested: true } as never,
          },
          unknown_provider: "not-an-object",
        },
      }),
    );

    expect(summary.status).toBe("failed");
    expect(summary.hasProviderError).toBe(true);
    expect(summary.goCache?.issueCategories).toEqual(["unknown"]);
    expect(summary.goCache?.hasUnknownIssue).toBe(true);
  });

  it("keeps a raw-only Go failure visible as partial", () => {
    const summary = storageRunSummary(
      makeRun({
        state: "failed",
        result: {
          go_cache: {
            error: "permission denied",
            result: { reclaimed_bytes: 1024, bytes_after: null, partial: true },
          },
        },
        message: "go_cache: permission denied",
      }),
    );

    expect(summary.status).toBe("partial");
    expect(summary.goCache?.issueCategories).toEqual(["other"]);
    expect(summary.hasProviderError).toBe(true);
  });

  it("keeps an unknown raw cause visible beside a budget code", () => {
    const summary = storageRunSummary(
      progressRun({
        result: {
          go_cache: {
            result: {
              reclaimed_bytes: 2 * GIB,
              bytes_after: null,
              reason: "cleanup_in_progress",
              errors: ["entry_limit_reached"],
              partial: true,
            },
            error: "entry_limit_reached, permission denied",
          },
        },
        message: `${BUDGET_MESSAGE}, permission denied`,
      }),
    );

    expect(summary.status).toBe("partial");
    expect(summary.goCache?.issueCategories).toEqual(["entry_limit", "other"]);
  });

  it("keeps a genuine error visible beside known issue codes", () => {
    const summary = storageRunSummary(
      progressRun({
        result: {
          go_cache: {
            result: {
              reclaimed_bytes: 0,
              bytes_after: null,
              reason: "incomplete_scan",
              errors: ["entry_limit_reached", "weird_future_code"],
            },
            error: "entry_limit_reached, weird_future_code",
          },
        },
        message: `${BUDGET_MESSAGE}, weird_future_code`,
      }),
    );

    expect(summary.status).toBe("failed");
    expect(summary.goCache?.hasUnknownIssue).toBe(true);
  });
});

describe("storageGoCacheResultSummary evidence", () => {
  it("keeps partial evidence when no issue codes are retained", () => {
    const outcome = storageGoCacheResultSummary({
      go_cache: { result: { reclaimed_bytes: 1024, bytes_after: null, partial: true } },
    });

    expect(outcome?.partial).toBe(true);
    expect(outcome?.issueCategories).toEqual([]);
  });

  it("keeps a raw-only permission failure visible", () => {
    const outcome = storageGoCacheResultSummary({
      go_cache: {
        error: "permission denied",
        result: { reclaimed_bytes: 1024, bytes_after: null, partial: true },
      },
    });

    expect(outcome?.partial).toBe(true);
    expect(outcome?.issueCategories).toEqual(["other"]);
  });

  it("distinguishes read and delete failures from identity changes", () => {
    const read = storageGoCacheResultSummary({
      go_cache: {
        result: { reclaimed_bytes: 0, errors: ["entry_read_failed"] },
        error: "permission denied\nentry_read_failed",
      },
    });
    const deleted = storageGoCacheResultSummary({
      go_cache: {
        result: { reclaimed_bytes: 0, errors: ["entry_delete_failed"] },
        error: "permission denied\nentry_delete_failed",
      },
    });

    expect(read?.issueCategories).toEqual(["unreadable"]);
    expect(deleted?.issueCategories).toEqual(["delete_failed"]);
  });

  it("keeps an unrelated read failure visible beside a parent change", () => {
    const outcome = storageGoCacheResultSummary({
      go_cache: {
        result: { reclaimed_bytes: 1, errors: ["cache_parent_changed", "entry_read_failed"] },
        error: "go-cache parent changed after scan\npermission denied\nentry_read_failed",
      },
    });

    expect(outcome?.issueCategories).toEqual(["entries_changed", "unreadable"]);
  });

  it("keeps the incident deduplicated when legacy codes accompany parent change", () => {
    const outcome = storageGoCacheResultSummary({
      go_cache: {
        result: { reclaimed_bytes: 1, errors: ["entry_delete_failed", "entry_read_failed"] },
        error: "go-cache parent changed after scan\nentry_delete_failed",
      },
    });

    expect(outcome?.issueCategories).toEqual(["entries_changed"]);
  });
});

describe("storageRunSummary compatibility", () => {
  it("reads legacy nested result envelopes", () => {
    const summary = storageRunSummary(
      makeRun({
        state: "failed",
        result: {
          result: {
            go_cache: {
              result: { reclaimed_bytes: 1 * GIB, bytes_after: 2 * GIB },
              error: PARENT_CHANGE_ERROR,
            },
          },
        },
        message: `go_cache: ${PARENT_CHANGE_ERROR}`,
      }),
    );

    expect(summary.status).toBe("partial");
    expect(summary.goCache?.issueCategories).toEqual(["entries_changed"]);
  });

  it("preserves queued, running, cancelled, and busy states", () => {
    for (const state of ["queued", "running", "cancelled", "skipped_busy"] as const) {
      expect(storageRunSummary(makeRun({ state })).status).toBe(state);
    }
  });

  it("falls back to an unknown status for an unrecognized stored state", () => {
    expect(storageRunSummary(makeRun({ state: "future_state" as never })).status).toBe("unknown");
  });

  it("keeps a successful run successful and a bare failure failed", () => {
    expect(storageRunSummary(makeRun({ state: "succeeded", result: {} })).status).toBe("succeeded");
    expect(storageRunSummary(makeRun({ state: "failed", result: {} })).status).toBe("failed");
  });

  it("recognizes useful non-Go work only from byte evidence", () => {
    expect(
      storageRunSummary(
        makeRun({ state: "succeeded", result: { workspaces: { result: { reclaimed_bytes: 4 } } } }),
      ).status,
    ).toBe("succeeded");
    expect(
      storageRunSummary(
        makeRun({
          state: "failed",
          result: { workspaces: { result: { quarantine: 2 }, error: "purge failed" } },
        }),
      ).status,
    ).toBe("failed");
    expect(
      storageRunSummary(
        makeRun({
          state: "failed",
          result: { workspaces: { result: { reclaimed_bytes: 4, errors: ["io_error"] } } },
        }),
      ).status,
    ).toBe("partial");
  });

  it("does not infer progress from an English parent-change message alone", () => {
    const summary = storageRunSummary(
      makeRun({
        state: "failed",
        result: { go_cache: { result: { reclaimed_bytes: 0 } } },
        message: `go_cache: ${PARENT_CHANGE_ERROR}`,
      }),
    );

    expect(summary.status).toBe("failed");
  });
});

describe("storageGoCacheResultSummary", () => {
  it("returns null without Go-cache result bytes", () => {
    expect(storageGoCacheResultSummary({ temporary_artifacts: { result: {} } })).toBeNull();
    expect(
      storageGoCacheResultSummary({ go_cache: { result: { reclaimed_bytes: "2" } } }),
    ).toBeNull();
  });

  it("keeps an explicitly missing remaining measurement unknown", () => {
    const outcome = storageGoCacheResultSummary({
      go_cache: { result: { reclaimed_bytes: 0, bytes_after: null } },
    });

    expect(outcome?.bytesAfter).toBeNull();
    expect(outcome?.reclaimedBytes).toBe(0);
  });

  it("reports an unknown issue code without inventing a count", () => {
    const outcome = storageGoCacheResultSummary({
      go_cache: {
        result: { reclaimed_bytes: 0, errors: ["future_code", "future_code"] },
      },
    });

    expect(outcome?.issueCategories).toEqual(["unknown"]);
    expect(outcome?.hasUnknownIssue).toBe(true);
  });
});

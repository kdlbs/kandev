import { describe, expect, it } from "vitest";
import type {
  StorageQuarantineEntry,
  StorageSummary,
  StorageSummaryPartial,
} from "@/lib/types/system";
import { quarantineTotalBytes, storageAnalysisTotal } from "./storage-totals";

const completeSummary: StorageSummary = {
  workspaces: {
    total_bytes: 10,
    active_bytes: 8,
    candidate_bytes: 9,
  },
  go_cache: {
    path: "/var/cache/kandev/go-build",
    size_bytes: 3,
    owned: true,
    unmanaged_path: "/root/.cache/go-build",
    unmanaged_size_bytes: 4,
  },
  quarantine: { count: 1, size_bytes: 2 },
  temporary_artifacts: {
    available: true,
    total_count: 1,
    total_bytes: 12,
    active_count: 0,
    active_bytes: 0,
    protected_count: 0,
    protected_bytes: 0,
    stale_count: 1,
    stale_bytes: 12,
    skipped_count: 0,
  },
  docker: {
    available: true,
    managed_container_count: 1,
    managed_container_bytes: 5,
    image_layer_bytes: 6,
    build_cache_bytes: 7,
    unused_image_bytes: 11,
  },
  database: {
    status: "measured",
    size_bytes: 1,
    path: "/data/kandev.db",
    included_in_total: true,
  },
  database_backups: {
    status: "measured",
    size_bytes: 2,
    path: "/data/backups",
    included_in_total: true,
  },
};

describe("storageAnalysisTotal", () => {
  it("sums non-overlapping top-level measurements", () => {
    expect(storageAnalysisTotal(completeSummary)).toEqual({ bytes: 52, partial: false });
  });

  it("excludes workspace subsets and unused Docker images", () => {
    const withoutSubsets = storageAnalysisTotal(completeSummary);
    const summary = {
      ...completeSummary,
      workspaces: { ...completeSummary.workspaces, active_bytes: 0, candidate_bytes: 0 },
      docker: { ...completeSummary.docker, unused_image_bytes: 0 },
    };

    expect(storageAnalysisTotal(summary)).toEqual(withoutSubsets);
  });

  it("marks unavailable top-level measurements as partial", () => {
    const summary: StorageSummary = {
      ...completeSummary,
      workspaces: { available: false, warning: "workspace inventory unavailable" },
      quarantine: { available: false, warning: "quarantine unavailable" },
      go_cache: { ...completeSummary.go_cache, size_bytes: undefined },
      docker: { ...completeSummary.docker, available: false },
    };

    expect(storageAnalysisTotal(summary)).toEqual({ bytes: 19, partial: true });
  });

  it("does not require a user cache when no distinct path is reported", () => {
    const summary: StorageSummary = {
      ...completeSummary,
      go_cache: {
        ...completeSummary.go_cache,
        unmanaged_path: undefined,
        unmanaged_size_bytes: undefined,
      },
    };

    expect(storageAnalysisTotal(summary)).toEqual({ bytes: 48, partial: false });
  });

  it("sums only completed values in a partial first-scan summary", () => {
    const summary: StorageSummaryPartial = {
      workspaces: { total_bytes: 12 },
      quarantine: null,
    };

    expect(storageAnalysisTotal(summary)).toEqual({ bytes: 12, partial: true });
  });
});

describe("storageAnalysisTotal database cases", () => {
  it("adds measured database and backup bytes exactly once", () => {
    expect(
      storageAnalysisTotal({
        ...completeSummary,
        database: completeSummary.database,
        database_backups: completeSummary.database_backups,
      }),
    ).toEqual({ bytes: 52, partial: false });
  });

  it("does not add overlap or not-applicable database measurements", () => {
    expect(
      storageAnalysisTotal({
        ...completeSummary,
        database: {
          status: "measured",
          size_bytes: 9,
          included_in_total: false,
          reason: "overlaps_existing_source",
        },
        database_backups: {
          status: "not_applicable",
          included_in_total: false,
          reason: "unsupported_driver",
        },
      }),
    ).toEqual({ bytes: 49, partial: false });
  });

  it("excludes the informational system temporary footprint from Total counted", () => {
    expect(
      storageAnalysisTotal({
        ...completeSummary,
        system_temporary: {
          status: "measured",
          size_bytes: 500,
          included_in_total: false,
          roots: [],
        },
      }),
    ).toEqual({ bytes: 52, partial: false });
  });

  it("adds only the counted portion of a partial database footprint", () => {
    expect(
      storageAnalysisTotal({
        ...completeSummary,
        database_backups: {
          status: "measured",
          size_bytes: 110,
          counted_size_bytes: 100,
          included_in_total: true,
          reason: "partially_overlaps_existing_source",
        },
      }),
    ).toEqual({ bytes: 150, partial: false });
  });

  it("marks unavailable or missing database measurements as partial", () => {
    expect(
      storageAnalysisTotal({
        database: {
          status: "unavailable",
          included_in_total: false,
          reason: "measurement_failed",
        },
        database_backups: undefined,
      }),
    ).toEqual({ bytes: 0, partial: true });
  });
});

describe("quarantineTotalBytes", () => {
  it("sums every listed entry, including zero-byte entries", () => {
    const entries = [
      { size_bytes: 5 },
      { size_bytes: 0 },
      { size_bytes: 8 },
    ] as StorageQuarantineEntry[];

    expect(quarantineTotalBytes(entries)).toBe(13);
    expect(quarantineTotalBytes([])).toBe(0);
  });
});

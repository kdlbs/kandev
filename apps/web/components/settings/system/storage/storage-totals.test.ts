import { describe, expect, it } from "vitest";
import type { StorageQuarantineEntry, StorageSummary } from "@/lib/types/system";
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
  docker: {
    available: true,
    managed_container_count: 1,
    managed_container_bytes: 5,
    image_layer_bytes: 6,
    build_cache_bytes: 7,
    unused_image_bytes: 11,
  },
};

describe("storageAnalysisTotal", () => {
  it("sums non-overlapping top-level measurements", () => {
    expect(storageAnalysisTotal(completeSummary)).toEqual({ bytes: 37, partial: false });
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

    expect(storageAnalysisTotal(summary)).toEqual({ bytes: 4, partial: true });
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

    expect(storageAnalysisTotal(summary)).toEqual({ bytes: 33, partial: false });
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

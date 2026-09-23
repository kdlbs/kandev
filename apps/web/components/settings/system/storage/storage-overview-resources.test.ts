import { describe, expect, it } from "vitest";
import type { StorageOverviewResponse, StorageSummary } from "@/lib/types/system";
import {
  storageResources,
  type StorageResource,
  type Translate,
} from "./storage-overview-resources";

const translate: Translate = (key) => key;
const SYSTEM_TEMPORARY_ID = "system-temporary";
const GO_CACHE_ID = "go-cache";
const TEMPORARY_ARTIFACTS_ID = "temporary-artifacts";

function overview(summary: StorageSummary): StorageOverviewResponse {
  return {
    settings: {} as StorageOverviewResponse["settings"],
    capabilities: {
      managed_go_cache_path: "/cache/go-build",
      docker_host: "",
    } as StorageOverviewResponse["capabilities"],
    summary,
    analyzed_at: null,
    analysis: {
      state: "ready",
      partial_summary: null,
      progress: { sources: {} },
    } as StorageOverviewResponse["analysis"],
    last_run: null,
  };
}

function resourceField(resource: StorageResource, field: string): unknown {
  return (resource as unknown as Record<string, unknown>)[field];
}

// eslint-disable-next-line max-lines-per-function -- the fixture covers the complete sorted resource set.
describe("storageResources relative measurements", () => {
  it("sorts measured rows by bytes and scales them against the largest row", () => {
    const resources = storageResources(
      translate,
      overview({
        workspaces: { total_bytes: 80 },
        database: { status: "measured", size_bytes: 10, included_in_total: true },
        database_backups: { status: "measured", size_bytes: 10, included_in_total: true },
        quarantine: { available: true, count: 0, size_bytes: 0 },
        system_temporary: {
          status: "partial",
          size_bytes: 100,
          included_in_total: false,
          roots: [],
        },
        go_cache: { size_bytes: 0 },
        temporary_artifacts: { available: true },
        docker: {
          available: false,
          build_cache_bytes: 0,
          unused_image_bytes: 0,
          managed_container_count: 0,
          managed_container_bytes: 0,
        },
      }),
    );

    expect(resources.map((resource) => resource.id)).toEqual([
      SYSTEM_TEMPORARY_ID,
      "workspaces",
      "database",
      "database-backups",
      "quarantine",
      GO_CACHE_ID,
      TEMPORARY_ARTIFACTS_ID,
      "managed-containers",
      "docker-image-layers",
      "docker-build-cache",
      "docker-unused-images",
    ]);

    const byId = new Map(resources.map((resource) => [resource.id, resource]));
    expect(resourceField(byId.get(SYSTEM_TEMPORARY_ID)!, "sizeBytes")).toBe(100);
    expect(resourceField(byId.get(SYSTEM_TEMPORARY_ID)!, "partial")).toBe(true);
    expect(resourceField(byId.get(SYSTEM_TEMPORARY_ID)!, "barPercent")).toBe(100);
    expect(resourceField(byId.get("workspaces")!, "barPercent")).toBe(80);
    expect(resourceField(byId.get("database")!, "barPercent")).toBe(10);
    expect(resourceField(byId.get("quarantine")!, "barPercent")).toBe(0);
    expect(resourceField(byId.get(TEMPORARY_ARTIFACTS_ID)!, "sizeBytes")).toBeUndefined();
    expect(resourceField(byId.get(TEMPORARY_ARTIFACTS_ID)!, "barPercent")).toBeUndefined();
  });

  it("keeps missing and invalid measurements distinct from measured zero", () => {
    const resources = storageResources(
      translate,
      overview({
        workspaces: {},
        database: { status: "measured", size_bytes: Number.NaN, included_in_total: true },
        database_backups: { status: "unavailable", included_in_total: false },
        quarantine: { available: true, count: 0, size_bytes: 0 },
        go_cache: { size_bytes: -1 },
        temporary_artifacts: { available: true, total_bytes: undefined },
        docker: {
          available: false,
          build_cache_bytes: 0,
          unused_image_bytes: 0,
          managed_container_count: 0,
          managed_container_bytes: 0,
        },
      }),
    );

    const byId = new Map(resources.map((resource) => [resource.id, resource]));
    expect(resourceField(byId.get("quarantine")!, "sizeBytes")).toBe(0);
    expect(resourceField(byId.get("quarantine")!, "barPercent")).toBe(0);
    expect(resourceField(byId.get("workspaces")!, "sizeBytes")).toBeUndefined();
    expect(resourceField(byId.get("database")!, "sizeBytes")).toBeUndefined();
    expect(resourceField(byId.get(GO_CACHE_ID)!, "sizeBytes")).toBeUndefined();
    expect(resourceField(byId.get(TEMPORARY_ARTIFACTS_ID)!, "sizeBytes")).toBeUndefined();
    expect(byId.get("workspaces")?.value).toBe("system:storageUnavailableValue");
  });

  it("maps every optional and subset measurement from numeric fields", () => {
    const resources = storageResources(
      translate,
      overview({
        workspaces: { total_bytes: 11 },
        database: { status: "measured", size_bytes: 10, included_in_total: true },
        database_backups: { status: "measured", size_bytes: 9, included_in_total: true },
        quarantine: { available: true, count: 1, size_bytes: 8 },
        system_temporary: {
          status: "measured",
          size_bytes: 7,
          included_in_total: false,
          roots: [],
        },
        go_cache: {
          size_bytes: 6,
          unmanaged_path: "/home/user/.cache/go-build",
          unmanaged_size_bytes: 5,
        },
        temporary_artifacts: { available: true, total_bytes: 4 },
        docker: {
          available: true,
          managed_container_count: 1,
          managed_container_bytes: 3,
          image_layer_bytes: 2,
          build_cache_bytes: 1,
          unused_image_bytes: 0,
        },
      }),
    );

    const sizes = new Map(
      resources.map((resource) => [resource.id, resourceField(resource, "sizeBytes")]),
    );
    expect(sizes).toEqual(
      new Map([
        ["workspaces", 11],
        ["database", 10],
        ["database-backups", 9],
        ["quarantine", 8],
        [SYSTEM_TEMPORARY_ID, 7],
        [GO_CACHE_ID, 6],
        ["unmanaged-go-cache", 5],
        ["temporary-artifacts", 4],
        ["managed-containers", 3],
        ["docker-image-layers", 2],
        ["docker-build-cache", 1],
        ["docker-unused-images", 0],
      ]),
    );
  });
});

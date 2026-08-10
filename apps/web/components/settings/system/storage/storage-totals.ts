import type { StorageQuarantineEntry, StorageSummary } from "@/lib/types/system";

export interface StorageAnalysisTotal {
  bytes: number;
  partial: boolean;
}

function isMeasuredBytes(value: number | undefined): value is number {
  return value !== undefined && Number.isFinite(value) && value >= 0;
}

export function storageAnalysisTotal(summary: StorageSummary): StorageAnalysisTotal {
  const total: StorageAnalysisTotal = { bytes: 0, partial: false };
  const addMeasurement = (value: number | undefined, available = true) => {
    if (!available || !isMeasuredBytes(value)) {
      total.partial = true;
      return;
    }
    total.bytes += value;
  };

  addMeasurement(summary.workspaces.total_bytes, summary.workspaces.available !== false);
  addMeasurement(
    summary.quarantine.available === false ? undefined : summary.quarantine.size_bytes,
  );
  addMeasurement(summary.go_cache.size_bytes, summary.go_cache.available !== false);
  if (summary.go_cache.unmanaged_path) {
    addMeasurement(summary.go_cache.unmanaged_size_bytes);
  }
  addMeasurement(
    summary.temporary_artifacts.total_bytes,
    summary.temporary_artifacts.available !== false,
  );

  if (summary.docker.available) {
    addMeasurement(summary.docker.managed_container_bytes);
    addMeasurement(summary.docker.image_layer_bytes);
    addMeasurement(summary.docker.build_cache_bytes);
  } else {
    addMeasurement(undefined, false);
    addMeasurement(undefined, false);
    addMeasurement(undefined, false);
  }

  return total;
}

export function quarantineTotalBytes(entries: StorageQuarantineEntry[]): number {
  return entries.reduce(
    (total, entry) => total + (isMeasuredBytes(entry.size_bytes) ? entry.size_bytes : 0),
    0,
  );
}

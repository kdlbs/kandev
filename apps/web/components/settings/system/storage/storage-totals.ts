import type {
  StorageQuarantineEntry,
  StorageFootprintMeasurement,
  StorageSummary,
  StorageSummaryPartial,
} from "@/lib/types/system";

export interface StorageAnalysisTotal {
  bytes: number;
  partial: boolean;
}

function addDatabaseMeasurement(
  total: StorageAnalysisTotal,
  measurement: StorageFootprintMeasurement | null | undefined,
): void {
  if (!measurement) {
    total.partial = true;
    return;
  }
  if (measurement.status === "not_applicable") {
    return;
  }
  if (measurement.status !== "measured" || !isMeasuredBytes(measurement.size_bytes)) {
    total.partial = true;
    return;
  }
  if (measurement.included_in_total === true) {
    const countedBytes = measurement.counted_size_bytes ?? measurement.size_bytes;
    if (!isMeasuredBytes(countedBytes)) {
      total.partial = true;
      return;
    }
    total.bytes += countedBytes;
    return;
  }
  if (measurement.included_in_total !== false) {
    total.partial = true;
  }
}

function isMeasuredBytes(value: number | undefined): value is number {
  return value !== undefined && Number.isFinite(value) && value >= 0;
}

export function storageAnalysisTotal(
  summary: StorageSummary | StorageSummaryPartial,
): StorageAnalysisTotal {
  const total: StorageAnalysisTotal = { bytes: 0, partial: false };
  const addMeasurement = (value: number | undefined, available = true) => {
    if (!available || !isMeasuredBytes(value)) {
      total.partial = true;
      return;
    }
    total.bytes += value;
  };

  addMeasurement(
    summary.workspaces?.total_bytes,
    summary.workspaces != null && summary.workspaces.available !== false,
  );
  addMeasurement(
    summary.quarantine?.available === false ? undefined : summary.quarantine?.size_bytes,
    summary.quarantine != null && summary.quarantine.available !== false,
  );
  addMeasurement(
    summary.go_cache?.size_bytes,
    summary.go_cache != null && summary.go_cache.available !== false,
  );
  if (summary.go_cache?.unmanaged_path) {
    addMeasurement(summary.go_cache.unmanaged_size_bytes);
  }
  addMeasurement(
    summary.temporary_artifacts?.total_bytes,
    summary.temporary_artifacts != null && summary.temporary_artifacts.available !== false,
  );

  if (summary.docker?.available) {
    addMeasurement(summary.docker.managed_container_bytes);
    addMeasurement(summary.docker.image_layer_bytes);
    addMeasurement(summary.docker.build_cache_bytes);
  } else {
    addMeasurement(undefined, false);
    addMeasurement(undefined, false);
    addMeasurement(undefined, false);
  }

  addDatabaseMeasurement(total, summary.database);
  addDatabaseMeasurement(total, summary.database_backups);

  return total;
}

export function quarantineTotalBytes(entries: StorageQuarantineEntry[]): number {
  return entries.reduce(
    (total, entry) => total + (isMeasuredBytes(entry.size_bytes) ? entry.size_bytes : 0),
    0,
  );
}

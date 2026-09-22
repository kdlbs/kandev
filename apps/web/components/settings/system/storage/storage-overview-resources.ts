import type {
  StorageFootprintMeasurement,
  StorageOverviewResponse,
  StorageQuarantineSummary,
  StorageSourceProgress,
  StorageSystemTemporarySummary,
  StorageSummaryPartial,
  StorageTemporaryArtifactsSummary,
} from "@/lib/types/system";
import { formatGigabytes } from "./storage-units";

/**
 * Resource-row copy is built in plain functions so each caller can provide its
 * current translation function and rows update after a locale switch.
 */
export type Translate = (key: string, options?: Record<string, unknown>) => string;
export const TEMPORARY_ARTIFACTS_RESOURCE_ID = "temporary-artifacts";
export const DATABASE_RESOURCE_ID = "database";
export const DATABASE_BACKUPS_RESOURCE_ID = "database-backups";
export const SYSTEM_TEMPORARY_RESOURCE_ID = "system-temporary";

export interface StorageResource {
  id: string;
  label: string;
  value: string;
  detail: string;
  detailLines?: string[];
  warning?: string;
  source?: string;
  sizeBytes?: number;
  barPercent?: number;
  partial?: boolean;
}

const STORAGE_UNAVAILABLE_VALUE_KEY = "system:storageUnavailableValue";
// i18n-exempt: stable backend error marker from older storage snapshots.
const LEGACY_DEADLINE_WARNING = "context deadline exceeded";
const TEMPORARY_WARNING_LIMIT = 10;

function measuredBytes(value: number | undefined): number | undefined {
  if (value === undefined || !Number.isFinite(value) || value < 0) return undefined;
  return value;
}

function analysisSourceText(t: Translate, progress?: StorageSourceProgress): string {
  if (!progress || progress.state === "pending") {
    return t("system:storageAnalysisSourcePending");
  }
  if (progress.state === "scanning") {
    if (progress.total_items !== undefined) {
      return t("system:storageAnalysisSourceScanning", {
        completed: progress.completed_items,
        total: progress.total_items,
      });
    }
    return t("system:storageAnalysisSourceScanningUnknown");
  }
  if (progress.state === "failed") {
    return progress.error
      ? t("system:storageAnalysisSourceFailedWithError", { error: progress.error })
      : t("system:storageAnalysisSourceFailed");
  }
  return t("system:storageAnalysisSourceComplete");
}

function pendingStorageResource(
  t: Translate,
  id: string,
  label: string,
  progress?: StorageSourceProgress,
  source = id,
): StorageResource {
  const status = analysisSourceText(t, progress);
  return { id, label, value: status, detail: status, source };
}

function quarantineResource(t: Translate, summary: StorageQuarantineSummary): StorageResource {
  if (summary.available === false) {
    return {
      id: "quarantine",
      label: t("system:storageQuarantinedResources"),
      value: t(STORAGE_UNAVAILABLE_VALUE_KEY),
      detail: t("system:storageQuarantineUnmeasured"),
      warning: summary.warning,
    };
  }
  const sizeBytes = measuredBytes(summary.size_bytes);
  return {
    id: "quarantine",
    label: t("system:storageQuarantinedResources"),
    value: sizeBytes === undefined ? t(STORAGE_UNAVAILABLE_VALUE_KEY) : formatGigabytes(sizeBytes),
    detail: t("system:storageQuarantineMovedAside", { count: summary.count }),
    sizeBytes,
  };
}

function dockerMeasurement(
  t: Translate,
  available: boolean,
  value: number | undefined,
  detail: string,
): Pick<StorageResource, "value" | "detail" | "sizeBytes"> {
  if (!available) {
    return {
      value: t(STORAGE_UNAVAILABLE_VALUE_KEY),
      detail: t("system:storageDockerUnmeasured"),
    };
  }
  const sizeBytes = measuredBytes(value);
  return {
    value: sizeBytes === undefined ? t(STORAGE_UNAVAILABLE_VALUE_KEY) : formatGigabytes(sizeBytes),
    detail,
    sizeBytes,
  };
}

function temporaryArtifactsResource(
  t: Translate,
  summary: StorageTemporaryArtifactsSummary,
): StorageResource {
  const warnings = [summary.warning, ...(summary.warnings ?? [])].filter(Boolean).join(" · ");
  if (summary.available === false) {
    return {
      id: TEMPORARY_ARTIFACTS_RESOURCE_ID,
      label: t("system:storageTemporaryArtifacts"),
      value: t(STORAGE_UNAVAILABLE_VALUE_KEY),
      detail: t("system:storageTemporaryArtifactsUnavailable"),
      warning: warnings || undefined,
    };
  }
  const sizeBytes = measuredBytes(summary.total_bytes);
  const stale = summary.stale_count ?? 0;
  const active = summary.active_count ?? 0;
  const protectedCount = summary.protected_count ?? 0;
  const staleBytes =
    summary.stale_bytes === undefined
      ? undefined
      : t("system:storageTemporaryArtifactsStaleBytes", {
          value: formatGigabytes(summary.stale_bytes),
        });
  return {
    id: TEMPORARY_ARTIFACTS_RESOURCE_ID,
    label: t("system:storageTemporaryArtifacts"),
    value: sizeBytes === undefined ? t(STORAGE_UNAVAILABLE_VALUE_KEY) : formatGigabytes(sizeBytes),
    detail: [
      t("system:storageTemporaryArtifactsStaleCount", { count: stale }),
      staleBytes,
      t("system:storageTemporaryArtifactsActiveCount", { count: active }),
      t("system:storageTemporaryArtifactsProtectedCount", { count: protectedCount }),
    ]
      .filter(Boolean)
      .join(" · "),
    warning: warnings || undefined,
    sizeBytes,
  };
}

function workspaceResource(
  t: Translate,
  workspace: StorageSummaryPartial["workspaces"],
  progress?: StorageSourceProgress,
): StorageResource {
  if (!workspace) {
    return pendingStorageResource(t, "workspaces", t("system:storageTaskWorkspaces"), progress);
  }
  const sizeBytes =
    workspace.available === false ? undefined : measuredBytes(workspace.total_bytes);
  return {
    id: "workspaces",
    label: t("system:storageTaskWorkspaces"),
    value: sizeBytes === undefined ? t(STORAGE_UNAVAILABLE_VALUE_KEY) : formatGigabytes(sizeBytes),
    detail: t("system:storageWorkspacesDetail", {
      reclaimable: formatGigabytes(workspace.candidate_bytes ?? 0),
      active: formatGigabytes(workspace.active_bytes ?? 0),
    }),
    warning: workspace.warning,
    source: "workspaces",
    sizeBytes,
  };
}

function quarantineResourceOrPending(
  t: Translate,
  quarantine: StorageSummaryPartial["quarantine"],
  progress?: StorageSourceProgress,
): StorageResource {
  if (!quarantine) {
    return pendingStorageResource(
      t,
      "quarantine",
      t("system:storageQuarantinedResources"),
      progress,
    );
  }
  return { ...quarantineResource(t, quarantine), source: "quarantine" };
}

function databaseResource(
  t: Translate,
  measurement: StorageFootprintMeasurement | null | undefined,
  progress: StorageSourceProgress | undefined,
  options: {
    id: string;
    label: string;
    detailKey: string;
    unknownDetailKey: string;
    unavailableDetailKey: string;
    notApplicableDetailKey: string;
    source: string;
  },
): StorageResource {
  if (!measurement) {
    if (progress?.state === "pending" || progress?.state === "scanning") {
      return pendingStorageResource(t, options.id, options.label, progress, options.source);
    }
    return {
      id: options.id,
      label: options.label,
      value: t(STORAGE_UNAVAILABLE_VALUE_KEY),
      detail: t(options.unknownDetailKey),
      source: options.source,
    };
  }
  switch (measurement.status) {
    case "unavailable":
      return {
        id: options.id,
        label: options.label,
        value: t(STORAGE_UNAVAILABLE_VALUE_KEY),
        detail: [t(options.unavailableDetailKey), measurement.path].filter(Boolean).join(" · "),
        warning: measurement.warning,
        source: options.source,
      };
    case "not_applicable":
      return {
        id: options.id,
        label: options.label,
        value: t("system:storageNotApplicableValue"),
        detail: [t(options.notApplicableDetailKey), measurement.path].filter(Boolean).join(" · "),
        warning: measurement.warning,
        source: options.source,
      };
    case "measured": {
      const sizeBytes = measuredBytes(measurement.size_bytes);
      const detail = [
        t(options.detailKey),
        measurement.included_in_total === false
          ? t("system:storageDatabaseAlreadyCounted")
          : undefined,
        measurement.reason === "partially_overlaps_existing_source"
          ? t("system:storageDatabasePartiallyCounted")
          : undefined,
        measurement.path,
      ]
        .filter(Boolean)
        .join(" · ");
      return {
        id: options.id,
        label: options.label,
        value:
          sizeBytes === undefined ? t(STORAGE_UNAVAILABLE_VALUE_KEY) : formatGigabytes(sizeBytes),
        detail,
        warning: measurement.warning,
        source: options.source,
        sizeBytes,
      };
    }
  }
}

type DockerSummary = NonNullable<StorageSummaryPartial["docker"]>;

interface DockerResourceOptions {
  progress?: StorageSourceProgress;
  id: string;
  label: string;
  value: number | undefined;
  detail: string;
  warning?: string;
}

function dockerResource(
  t: Translate,
  docker: DockerSummary | null | undefined,
  options: DockerResourceOptions,
): StorageResource {
  if (!docker) {
    return pendingStorageResource(t, options.id, options.label, options.progress, "docker");
  }
  return {
    id: options.id,
    label: options.label,
    ...dockerMeasurement(t, docker.available === true, options.value, options.detail),
    warning: options.warning,
    source: "docker",
  };
}

function goCacheResources(
  t: Translate,
  goCache: StorageSummaryPartial["go_cache"],
  progress: StorageSourceProgress | undefined,
  managedPath: string,
): StorageResource[] {
  const resources: StorageResource[] = [];
  if (goCache) {
    const sizeBytes = goCache.available === false ? undefined : measuredBytes(goCache.size_bytes);
    resources.push({
      id: "go-cache",
      label: t("system:storageGoBuildCache"),
      value:
        sizeBytes === undefined ? t(STORAGE_UNAVAILABLE_VALUE_KEY) : formatGigabytes(sizeBytes),
      // A filesystem path from the API is never routed through the catalog.
      detail: goCache.path ?? managedPath,
      warning: goCache.warning,
      source: "go_cache",
      sizeBytes,
    });
  } else {
    resources.push(
      pendingStorageResource(t, "go-cache", t("system:storageGoBuildCache"), progress, "go_cache"),
    );
  }
  if (goCache?.unmanaged_path) {
    const sizeBytes =
      goCache.available === false ? undefined : measuredBytes(goCache.unmanaged_size_bytes);
    resources.push({
      id: "unmanaged-go-cache",
      label: t("system:storageUserGoBuildCache"),
      value:
        sizeBytes === undefined ? t(STORAGE_UNAVAILABLE_VALUE_KEY) : formatGigabytes(sizeBytes),
      detail: goCache.unmanaged_path,
      source: "go_cache",
      sizeBytes,
    });
  }
  return resources;
}

function temporaryArtifactsResourceOrPending(
  t: Translate,
  temporaryArtifacts: StorageSummaryPartial["temporary_artifacts"],
  progress?: StorageSourceProgress,
): StorageResource {
  if (!temporaryArtifacts) {
    return pendingStorageResource(
      t,
      "temporary-artifacts",
      t("system:storageTemporaryArtifacts"),
      progress,
      "temporary_artifacts",
    );
  }
  return { ...temporaryArtifactsResource(t, temporaryArtifacts), source: "temporary_artifacts" };
}

function temporaryRootStatus(
  t: Translate,
  status: StorageSystemTemporarySummary["status"],
): string {
  switch (status) {
    case "measured":
      return t("system:storageSystemTemporaryComplete");
    case "partial":
      return t("system:storageSystemTemporaryPartial");
    case "unavailable":
      return t("system:storageSystemTemporaryUnavailable");
    case "not_applicable":
      return t("system:storageSystemTemporaryNotApplicable");
  }
}

function temporaryRootLine(
  t: Translate,
  root: NonNullable<StorageSystemTemporarySummary["roots"]>[number],
): string {
  const path =
    root.requested_path === root.path
      ? root.path
      : t("system:storageSystemTemporaryResolvedRoot", {
          requested: root.requested_path,
          resolved: root.path,
        });
  let value: string;
  if (root.status === "unavailable" || root.status === "not_applicable") {
    value = temporaryRootStatus(t, root.status);
  } else if (root.size_bytes === undefined) {
    value = t(STORAGE_UNAVAILABLE_VALUE_KEY);
  } else {
    value = formatGigabytes(root.size_bytes);
  }
  const line = t("system:storageSystemTemporaryRoot", {
    path,
    size: value,
    status: temporaryRootStatus(t, root.status),
  });
  if (!root.skipped_count) return line;
  return [line, t("system:storageSystemTemporarySkippedCount", { count: root.skipped_count })].join(
    " · ",
  );
}

function temporarySummaryBytes(summary: StorageSystemTemporarySummary): number | undefined {
  if (summary.status === "unavailable" || summary.status === "not_applicable") {
    return undefined;
  }
  return measuredBytes(summary.size_bytes);
}

function temporarySummaryValue(t: Translate, summary: StorageSystemTemporarySummary): string {
  if (summary.status === "unavailable") {
    return t(STORAGE_UNAVAILABLE_VALUE_KEY);
  }
  if (summary.status === "not_applicable") {
    return t("system:storageNotApplicableValue");
  }
  const sizeBytes = temporarySummaryBytes(summary);
  if (sizeBytes === undefined) {
    return t(STORAGE_UNAVAILABLE_VALUE_KEY);
  }
  return formatGigabytes(sizeBytes);
}

function temporarySummaryDetail(
  t: Translate,
  status: StorageSystemTemporarySummary["status"],
): string {
  switch (status) {
    case "partial":
      return t("system:storageSystemTemporaryPartialDetail");
    case "unavailable":
      return t("system:storageSystemTemporaryUnavailableDetail");
    case "not_applicable":
      return t("system:storageSystemTemporaryNotApplicableDetail");
    case "measured":
      return t("system:storageSystemTemporaryCompleteDetail");
  }
}

function normalizeTemporaryWarnings(warnings: string[]): {
  values: string[];
  includesDeadline: boolean;
} {
  const values: string[] = [];
  let includesDeadline = false;
  for (const warning of warnings) {
    for (const line of warning.split(/\r?\n/)) {
      const value = line.trim();
      if (!value) continue;
      if (value === LEGACY_DEADLINE_WARNING) {
        includesDeadline = true;
        continue;
      }
      if (values.includes(value) || values.length >= TEMPORARY_WARNING_LIMIT) continue;
      values.push(value);
    }
  }
  return { values, includesDeadline };
}

function systemTemporaryResource(
  t: Translate,
  summary: StorageSystemTemporarySummary,
): StorageResource {
  const normalizedWarnings = normalizeTemporaryWarnings([
    ...(summary.warnings ?? []),
    ...summary.roots.flatMap((root) => root.warnings ?? []),
  ]);
  const includesDeadline =
    summary.reason === "deadline" ||
    summary.roots.some((root) => root.reason === "deadline") ||
    normalizedWarnings.includesDeadline;
  const value = temporarySummaryValue(t, summary);
  const statusDetail = temporarySummaryDetail(t, summary.status);
  const detailLines = summary.roots.map((root) => temporaryRootLine(t, root));
  if (includesDeadline) {
    detailLines.push(t("system:storageSystemTemporaryDeadlineDetail"));
  }
  return {
    id: SYSTEM_TEMPORARY_RESOURCE_ID,
    label: t("system:storageSystemTemporaryFolders"),
    value,
    detail: [t("system:storageSystemTemporaryInformational"), statusDetail].join(" "),
    detailLines,
    warning: normalizedWarnings.values.join(" · ") || undefined,
    source: "system_temporary",
    sizeBytes: temporarySummaryBytes(summary),
    partial: summary.status === "partial",
  };
}

function systemTemporaryResourceOrPending(
  t: Translate,
  summary: StorageSummaryPartial["system_temporary"],
  progress?: StorageSourceProgress,
): StorageResource {
  if (!summary) {
    return pendingStorageResource(
      t,
      SYSTEM_TEMPORARY_RESOURCE_ID,
      t("system:storageSystemTemporaryFolders"),
      progress,
      "system_temporary",
    );
  }
  return systemTemporaryResource(t, summary);
}

function dockerResources(
  t: Translate,
  docker: DockerSummary | null | undefined,
  progress: StorageSourceProgress | undefined,
  host: string,
): StorageResource[] {
  const dockerWarning = docker?.warnings?.join(" · ");
  const dockerHost = host || t("system:storageDefaultDockerHost");
  return [
    dockerResource(t, docker, {
      progress,
      id: "managed-containers",
      label: t("system:storageKandevContainers"),
      value: docker?.managed_container_bytes,
      detail: t("system:storageManagedContainerCount", {
        count: docker?.managed_container_count ?? 0,
      }),
      warning: dockerWarning,
    }),
    dockerResource(t, docker, {
      progress,
      id: "docker-image-layers",
      label: t("system:storageDockerImageLayers"),
      value: docker?.image_layer_bytes,
      detail: dockerHost,
      warning: dockerWarning,
    }),
    dockerResource(t, docker, {
      progress,
      id: "docker-build-cache",
      label: t("system:storageDockerBuildCache"),
      value: docker?.build_cache_bytes,
      detail: dockerHost,
      warning: dockerWarning,
    }),
    dockerResource(t, docker, {
      progress,
      id: "docker-unused-images",
      label: t("system:storageUnusedDockerImages"),
      value: docker?.unused_image_bytes,
      detail: t("system:storageUnusedImagesDetail"),
      warning: dockerWarning,
    }),
  ];
}

function sortAndScaleResources(resources: StorageResource[]): StorageResource[] {
  const ordered = resources
    .map((resource, index) => ({ resource, index }))
    .sort((left, right) => {
      if (left.resource.sizeBytes === undefined && right.resource.sizeBytes === undefined) {
        return left.index - right.index;
      }
      if (left.resource.sizeBytes === undefined) return 1;
      if (right.resource.sizeBytes === undefined) return -1;
      if (left.resource.sizeBytes !== right.resource.sizeBytes) {
        return right.resource.sizeBytes - left.resource.sizeBytes;
      }
      return left.index - right.index;
    });
  const maximum = ordered.reduce<number | undefined>((largest, entry) => {
    if (entry.resource.sizeBytes === undefined) return largest;
    return largest === undefined
      ? entry.resource.sizeBytes
      : Math.max(largest, entry.resource.sizeBytes);
  }, undefined);
  return ordered.map(({ resource }) => {
    if (resource.sizeBytes === undefined || maximum === undefined) return resource;
    const barPercent = maximum === 0 ? 0 : Math.min(100, (resource.sizeBytes / maximum) * 100);
    return { ...resource, barPercent };
  });
}

export function storageResources(
  t: Translate,
  overview: StorageOverviewResponse,
): StorageResource[] {
  const summary = overview.summary ?? overview.analysis.partial_summary ?? {};
  const progress = overview.analysis.progress.sources;
  return sortAndScaleResources([
    workspaceResource(t, summary.workspaces, progress.workspaces),
    databaseResource(t, summary.database, progress.database, {
      id: DATABASE_RESOURCE_ID,
      label: t("system:storageDatabase"),
      detailKey: "system:storageDatabaseDetail",
      unknownDetailKey: "system:storageDatabaseUnknown",
      unavailableDetailKey: "system:storageDatabaseUnavailable",
      notApplicableDetailKey: "system:storageDatabaseNotApplicable",
      source: "database",
    }),
    databaseResource(t, summary.database_backups, progress.database_backups, {
      id: DATABASE_BACKUPS_RESOURCE_ID,
      label: t("system:storageDatabaseBackups"),
      detailKey: "system:storageDatabaseBackupsDetail",
      unknownDetailKey: "system:storageDatabaseBackupsUnknown",
      unavailableDetailKey: "system:storageDatabaseBackupsUnavailable",
      notApplicableDetailKey: "system:storageDatabaseBackupsNotApplicable",
      source: "database_backups",
    }),
    quarantineResourceOrPending(t, summary.quarantine, progress.quarantine),
    systemTemporaryResourceOrPending(t, summary.system_temporary, progress.system_temporary),
    ...goCacheResources(
      t,
      summary.go_cache,
      progress.go_cache,
      overview.capabilities.managed_go_cache_path,
    ),
    temporaryArtifactsResourceOrPending(
      t,
      summary.temporary_artifacts,
      progress.temporary_artifacts,
    ),
    ...dockerResources(t, summary.docker, progress.docker, overview.capabilities.docker_host),
  ]);
}

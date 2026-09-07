import type {
  StorageOverviewResponse,
  StorageQuarantineSummary,
  StorageSourceProgress,
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

export interface StorageResource {
  id: string;
  label: string;
  value: string;
  detail: string;
  warning?: string;
  source?: string;
}

const STORAGE_UNAVAILABLE_VALUE_KEY = "system:storageUnavailableValue";

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
  return {
    id: "quarantine",
    label: t("system:storageQuarantinedResources"),
    value: formatGigabytes(summary.size_bytes),
    detail: t("system:storageQuarantineMovedAside", { count: summary.count }),
  };
}

function dockerMeasurement(
  t: Translate,
  available: boolean,
  value: string,
  detail: string,
): Pick<StorageResource, "value" | "detail"> {
  if (!available) {
    return {
      value: t(STORAGE_UNAVAILABLE_VALUE_KEY),
      detail: t("system:storageDockerUnmeasured"),
    };
  }
  return { value, detail };
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
    value:
      summary.total_bytes === undefined
        ? t(STORAGE_UNAVAILABLE_VALUE_KEY)
        : formatGigabytes(summary.total_bytes),
    detail: [
      t("system:storageTemporaryArtifactsStaleCount", { count: stale }),
      staleBytes,
      t("system:storageTemporaryArtifactsActiveCount", { count: active }),
      t("system:storageTemporaryArtifactsProtectedCount", { count: protectedCount }),
    ]
      .filter(Boolean)
      .join(" · "),
    warning: warnings || undefined,
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
  return {
    id: "workspaces",
    label: t("system:storageTaskWorkspaces"),
    value: formatGigabytes(workspace.total_bytes ?? 0),
    detail: t("system:storageWorkspacesDetail", {
      reclaimable: formatGigabytes(workspace.candidate_bytes ?? 0),
      active: formatGigabytes(workspace.active_bytes ?? 0),
    }),
    warning: workspace.warning,
    source: "workspaces",
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

type DockerSummary = NonNullable<StorageSummaryPartial["docker"]>;

interface DockerResourceOptions {
  progress?: StorageSourceProgress;
  id: string;
  label: string;
  value: string;
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
  const resources: StorageResource[] = [
    goCache
      ? {
          id: "go-cache",
          label: t("system:storageGoBuildCache"),
          value: formatGigabytes(goCache.size_bytes ?? 0),
          // A filesystem path from the API is never routed through the catalog.
          detail: goCache.path ?? managedPath,
          warning: goCache.warning,
          source: "go_cache",
        }
      : pendingStorageResource(
          t,
          "go-cache",
          t("system:storageGoBuildCache"),
          progress,
          "go_cache",
        ),
  ];
  if (goCache?.unmanaged_path) {
    resources.push({
      id: "unmanaged-go-cache",
      label: t("system:storageUserGoBuildCache"),
      value: formatGigabytes(goCache.unmanaged_size_bytes ?? 0),
      detail: goCache.unmanaged_path,
      source: "go_cache",
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
      value: formatGigabytes(docker?.managed_container_bytes ?? 0),
      detail: t("system:storageManagedContainerCount", {
        count: docker?.managed_container_count ?? 0,
      }),
      warning: dockerWarning,
    }),
    dockerResource(t, docker, {
      progress,
      id: "docker-image-layers",
      label: t("system:storageDockerImageLayers"),
      value: formatGigabytes(docker?.image_layer_bytes ?? 0),
      detail: dockerHost,
      warning: dockerWarning,
    }),
    dockerResource(t, docker, {
      progress,
      id: "docker-build-cache",
      label: t("system:storageDockerBuildCache"),
      value: formatGigabytes(docker?.build_cache_bytes ?? 0),
      detail: dockerHost,
      warning: dockerWarning,
    }),
    dockerResource(t, docker, {
      progress,
      id: "docker-unused-images",
      label: t("system:storageUnusedDockerImages"),
      value: formatGigabytes(docker?.unused_image_bytes ?? 0),
      detail: t("system:storageUnusedImagesDetail"),
      warning: dockerWarning,
    }),
  ];
}

export function storageResources(
  t: Translate,
  overview: StorageOverviewResponse,
): StorageResource[] {
  const summary = overview.summary ?? overview.analysis.partial_summary ?? {};
  const progress = overview.analysis.progress.sources;
  return [
    workspaceResource(t, summary.workspaces, progress.workspaces),
    quarantineResourceOrPending(t, summary.quarantine, progress.quarantine),
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
  ];
}

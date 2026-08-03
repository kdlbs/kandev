import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@kandev/ui/accordion";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import { IconChartPie, IconTrash } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { formatDateTime, formatRelative } from "@/lib/i18n/formats";
import type {
  StorageMaintenanceSettings,
  StorageOverviewResponse,
  StorageQuarantineSummary,
} from "@/lib/types/system";
import { StorageActionButton } from "./storage-action-button";
import { formatGigabytes } from "./storage-units";
import { storageAnalysisTotal } from "./storage-totals";

/**
 * Copy for the resource rows is built in plain functions rather than JSX, so
 * `i18next/no-literal-string` (mode `jsx-only`) never inspected any of it. Each
 * takes `t` from the caller's `useTranslation` instead of importing the
 * module-level one, so the rows re-render on a locale switch.
 */
type Translate = (key: string, options?: Record<string, unknown>) => string;

interface Props {
  overview: StorageOverviewResponse | null;
  settings?: StorageMaintenanceSettings;
  loading?: boolean;
  error?: string | null;
  disabledReason?: string;
  onRunGoCache: () => void;
}

interface StorageResource {
  id: string;
  label: string;
  value: string;
  detail: string;
  warning?: string;
}

function goCacheDisabledReason(
  t: Translate,
  overview: StorageOverviewResponse,
  pendingReason?: string,
  settings?: StorageMaintenanceSettings,
) {
  if (pendingReason) return pendingReason;
  if (overview.summary.go_cache.owned !== true) {
    return t("system:storageGoCacheNotOwned");
  }
  if (
    (overview.summary.go_cache.size_bytes ?? 0) <=
    (settings ?? overview.settings).go_cache.max_bytes
  ) {
    return t("system:storageGoCacheBelowLimit");
  }
  return undefined;
}

function quarantineResource(t: Translate, summary: StorageQuarantineSummary): StorageResource {
  if (summary.available === false) {
    return {
      id: "quarantine",
      label: t("system:storageQuarantinedResources"),
      value: t("system:storageUnavailableValue"),
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
      value: t("system:storageUnavailableValue"),
      detail: t("system:storageDockerUnmeasured"),
    };
  }
  return { value, detail };
}

function storageResources(t: Translate, overview: StorageOverviewResponse): StorageResource[] {
  const { summary } = overview;
  const dockerWarning = summary.docker.warnings?.join(" · ");
  // The Docker host is an address, not copy; the fallback naming the default is.
  const dockerHost = overview.capabilities.docker_host || t("system:storageDefaultDockerHost");
  return [
    {
      id: "workspaces",
      label: t("system:storageTaskWorkspaces"),
      value: formatGigabytes(summary.workspaces.total_bytes ?? 0),
      detail: t("system:storageWorkspacesDetail", {
        reclaimable: formatGigabytes(summary.workspaces.candidate_bytes ?? 0),
        active: formatGigabytes(summary.workspaces.active_bytes ?? 0),
      }),
      warning: summary.workspaces.warning,
    },
    quarantineResource(t, summary.quarantine),
    {
      id: "managed-containers",
      label: t("system:storageKandevContainers"),
      ...dockerMeasurement(
        t,
        summary.docker.available,
        formatGigabytes(summary.docker.managed_container_bytes ?? 0),
        t("system:storageManagedContainerCount", {
          count: summary.docker.managed_container_count ?? 0,
        }),
      ),
      warning: dockerWarning,
    },
    {
      id: "go-cache",
      label: t("system:storageGoBuildCache"),
      value: formatGigabytes(summary.go_cache.size_bytes ?? 0),
      // A filesystem path from the API — never routed through the catalog.
      detail: summary.go_cache.path ?? overview.capabilities.managed_go_cache_path,
      warning: summary.go_cache.warning,
    },
    ...(summary.go_cache.unmanaged_path
      ? [
          {
            id: "unmanaged-go-cache",
            label: t("system:storageUserGoBuildCache"),
            value: formatGigabytes(summary.go_cache.unmanaged_size_bytes ?? 0),
            detail: summary.go_cache.unmanaged_path,
          },
        ]
      : []),
    {
      id: "docker-image-layers",
      label: t("system:storageDockerImageLayers"),
      ...dockerMeasurement(
        t,
        summary.docker.available,
        formatGigabytes(summary.docker.image_layer_bytes ?? 0),
        dockerHost,
      ),
      warning: dockerWarning,
    },
    {
      id: "docker-build-cache",
      label: t("system:storageDockerBuildCache"),
      ...dockerMeasurement(
        t,
        summary.docker.available,
        formatGigabytes(summary.docker.build_cache_bytes),
        dockerHost,
      ),
      warning: dockerWarning,
    },
    {
      id: "docker-unused-images",
      label: t("system:storageUnusedDockerImages"),
      ...dockerMeasurement(
        t,
        summary.docker.available,
        formatGigabytes(summary.docker.unused_image_bytes),
        t("system:storageUnusedImagesDetail"),
      ),
      warning: dockerWarning,
    },
  ];
}

interface ResourceRowProps {
  resource: StorageResource;
  goCacheCleanupDisabledReason?: string;
  onRunGoCache: () => void;
}

function ResourceRow({ resource, goCacheCleanupDisabledReason, onRunGoCache }: ResourceRowProps) {
  const { t } = useTranslation();
  return (
    <AccordionItem value={resource.id} data-testid={`storage-resource-${resource.id}`}>
      <AccordionTrigger
        className="min-h-11 items-center px-3 no-underline"
        data-testid={`storage-resource-${resource.id}-trigger`}
      >
        <span className="min-w-0">
          <span className="block text-sm">{resource.label}</span>
          <span className="block text-xs font-normal text-muted-foreground">{resource.value}</span>
        </span>
      </AccordionTrigger>
      <AccordionContent className="px-3">
        <p className="break-all text-muted-foreground">{resource.detail}</p>
        {resource.warning && <p className="mt-2 break-words text-amber-600">{resource.warning}</p>}
        {resource.id === "go-cache" && (
          <StorageActionButton
            variant="outline"
            className="mt-3 w-full sm:w-auto"
            disabledReason={goCacheCleanupDisabledReason}
            onClick={onRunGoCache}
            data-testid="storage-go-cache-clean"
          >
            <IconTrash className="size-4" /> {t("system:storageCleanGoCache")}
          </StorageActionButton>
        )}
      </AccordionContent>
    </AccordionItem>
  );
}

export function StorageOverviewCard({
  overview,
  settings,
  loading,
  error,
  disabledReason,
  onRunGoCache,
}: Props) {
  const { t } = useTranslation();
  const isLoading = loading ?? overview === null;
  if (!overview) {
    return (
      <Card data-testid="storage-overview-card">
        <CardContent className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
          {isLoading && <Spinner className="size-4" data-testid="storage-overview-spinner" />}
          <span>
            {isLoading ? t("system:storageLoadingData") : t("system:storageSectionUnavailable")}
          </span>
          {error && <span className="break-words text-destructive">{error}</span>}
        </CardContent>
      </Card>
    );
  }
  const { summary } = overview;
  const analyzedAt = formatDateTime(overview.analyzed_at);
  const cleanupDisabledReason = goCacheDisabledReason(t, overview, disabledReason, settings);
  const resources = storageResources(t, overview);
  const total = storageAnalysisTotal(summary);
  return (
    <Card className="min-w-0" data-testid="storage-overview-card">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <IconChartPie className="size-4" /> {t("system:storageAnalysisTitle")}
          {isLoading && <Spinner className="size-4" data-testid="storage-overview-spinner" />}
          {!summary.docker.available && (
            <Badge variant="outline">{t("system:storageDockerUnavailableBadge")}</Badge>
          )}
        </CardTitle>
        <CardDescription>{t("system:storageAnalysisDescription")}</CardDescription>
        <time
          className="text-xs text-muted-foreground"
          dateTime={overview.analyzed_at}
          title={analyzedAt}
          aria-label={t("system:storageLastAnalyzed", { time: analyzedAt })}
        >
          {t("system:storageLastAnalyzed", { time: formatRelative(overview.analyzed_at) })}
        </time>
        <div
          className="flex flex-wrap items-center gap-2 text-xs"
          data-testid="storage-analysis-total"
        >
          <span className="font-medium">
            {t("system:storageTotalCounted", { size: formatGigabytes(total.bytes) })}
          </span>
          {total.partial && (
            <Badge variant="outline" data-testid="storage-analysis-total-partial">
              {t("system:storageTotalPartial")}
            </Badge>
          )}
        </div>
        {error && (
          <p className="break-words text-xs text-destructive" data-testid="storage-overview-error">
            {t("system:storageSectionUnavailable")}: {error}
          </p>
        )}
      </CardHeader>
      <CardContent className="min-w-0">
        <Accordion type="multiple" className="min-w-0">
          {resources.map((resource) => (
            <ResourceRow
              key={resource.id}
              resource={resource}
              goCacheCleanupDisabledReason={cleanupDisabledReason}
              onRunGoCache={onRunGoCache}
            />
          ))}
        </Accordion>
      </CardContent>
    </Card>
  );
}

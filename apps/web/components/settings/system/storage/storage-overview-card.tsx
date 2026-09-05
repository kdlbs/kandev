import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@kandev/ui/accordion";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { IconChartPie, IconTrash } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { formatDateTime, formatRelative, formatRelativeTime } from "@/lib/i18n/formats";
import type {
  StorageMaintenanceSettings,
  StorageOverviewResponse,
  StorageSummaryPartial,
} from "@/lib/types/system";
import { StorageActionButton } from "./storage-action-button";
import { StorageSettingHelp } from "./storage-setting-help";
import {
  storageResources,
  TEMPORARY_ARTIFACTS_RESOURCE_ID,
  type StorageResource,
  type Translate,
} from "./storage-overview-resources";
import { formatGigabytes } from "./storage-units";
import { storageAnalysisTotal } from "./storage-totals";

interface Props {
  overview: StorageOverviewResponse | null;
  settings?: StorageMaintenanceSettings;
  loading?: boolean;
  error?: string | null;
  disabledReason?: string;
  onRunGoCache: () => void;
  onRunTemporaryArtifacts?: () => void;
}

function goCacheDisabledReason(
  t: Translate,
  overview: StorageOverviewResponse,
  pendingReason?: string,
  settings?: StorageMaintenanceSettings,
) {
  if (pendingReason) return pendingReason;
  const summary = overview.summary ?? overview.analysis.partial_summary;
  const goCache = summary?.go_cache;
  if (!goCache) return t("system:storageAnalysisSourcePending");
  if (goCache.owned !== true) {
    return t("system:storageGoCacheNotOwned");
  }
  if ((goCache.size_bytes ?? 0) <= (settings ?? overview.settings).go_cache.max_bytes) {
    return t("system:storageGoCacheBelowLimit");
  }
  return undefined;
}

function temporaryArtifactsDisabledReason(
  t: Translate,
  overview: StorageOverviewResponse,
  pendingReason?: string,
) {
  if (pendingReason) return pendingReason;
  const summary =
    overview.summary?.temporary_artifacts ?? overview.analysis.partial_summary?.temporary_artifacts;
  if (!summary) return t("system:storageAnalysisSourcePending");
  if (overview.capabilities.temporary_artifacts_available !== true || summary.available === false) {
    return t("system:storageTemporaryArtifactsUnavailable");
  }
  if ((summary.stale_count ?? 0) === 0) {
    return t("system:storageTemporaryArtifactsNoStale");
  }
  return undefined;
}

interface ResourceRowProps {
  resource: StorageResource;
  goCacheCleanupDisabledReason?: string;
  onRunGoCache: () => void;
  temporaryArtifactsCleanupDisabledReason?: string;
  onRunTemporaryArtifacts: () => void;
}

function ResourceRow({
  resource,
  goCacheCleanupDisabledReason,
  onRunGoCache,
  temporaryArtifactsCleanupDisabledReason,
  onRunTemporaryArtifacts,
}: ResourceRowProps) {
  const { t } = useTranslation();
  return (
    <AccordionItem value={resource.id} data-testid={`storage-resource-${resource.id}`}>
      <AccordionTrigger
        className="min-h-11 items-center px-3 no-underline"
        data-testid={`storage-resource-${resource.id}-trigger`}
      >
        <span className="min-w-0">
          <span className="block text-sm">{resource.label}</span>
          <span
            className="block text-xs font-normal text-muted-foreground"
            data-testid={resource.source ? `storage-analysis-source-${resource.source}` : undefined}
          >
            {resource.value}
          </span>
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
        {resource.id === TEMPORARY_ARTIFACTS_RESOURCE_ID && (
          <StorageActionButton
            variant="outline"
            className="mt-3 w-full sm:w-auto"
            disabledReason={temporaryArtifactsCleanupDisabledReason}
            onClick={onRunTemporaryArtifacts}
            data-testid="storage-temporary-artifacts-clean"
          >
            <IconTrash className="size-4" /> {t("system:storageCleanTemporaryArtifacts")}
          </StorageActionButton>
        )}
      </AccordionContent>
    </AccordionItem>
  );
}

function formatScanDuration(t: Translate, milliseconds: number): string {
  const value = Math.max(0, Math.round(milliseconds));
  if (value >= 60_000) {
    return t("system:storageAnalysisDurationMinutes", { value: Math.round(value / 60_000) });
  }
  if (value >= 1_000) {
    return t("system:storageAnalysisDurationSeconds", { value: (value / 1_000).toFixed(1) });
  }
  return t("system:storageAnalysisDurationMilliseconds", { value });
}

function AnalysisTimingDisclosure({ overview }: { overview: StorageOverviewResponse }) {
  const { t } = useTranslation();
  const { analysis } = overview;
  if (analysis.state !== "ready" || !overview.analyzed_at || analysis.duration_ms == null) {
    return null;
  }
  const nextRefresh = analysis.refresh_due_at
    ? t("system:storageAnalysisNextRefresh", {
        absolute: formatDateTime(analysis.refresh_due_at),
        relative: formatRelativeTime(analysis.refresh_due_at),
      })
    : t("system:storageAnalysisNextRefreshUnknown");
  const disclosure = [
    t("system:storageAnalysisDuration", {
      duration: formatScanDuration(t, analysis.duration_ms),
    }),
    t("system:storageAnalysisCacheLifetime", {
      duration: formatScanDuration(t, analysis.cache_ttl_seconds * 1_000),
    }),
    nextRefresh,
    t("system:storageAnalysisTimingHelp"),
  ].join(" ");
  return (
    <TooltipProvider>
      <StorageSettingHelp
        label={t("system:storageAnalysisTimingTitle")}
        testId="storage-analysis-timing-help"
      >
        {disclosure}
      </StorageSettingHelp>
    </TooltipProvider>
  );
}

function EmptyStorageOverview({
  isInitialLoading,
  error,
}: {
  isInitialLoading: boolean;
  error?: string | null;
}) {
  const { t } = useTranslation();
  return (
    <Card data-testid="storage-overview-card">
      <CardContent className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
        {isInitialLoading && <Spinner className="size-4" data-testid="storage-overview-spinner" />}
        <span>
          {isInitialLoading
            ? t("system:storageLoadingData")
            : t("system:storageSectionUnavailable")}
        </span>
        {error && <span className="break-words text-destructive">{error}</span>}
      </CardContent>
    </Card>
  );
}

function AnalysisStatusTime({
  overview,
  analyzedAt,
}: {
  overview: StorageOverviewResponse;
  analyzedAt: string | null;
}) {
  const { t } = useTranslation();
  if (analyzedAt && overview.analyzed_at) {
    return (
      <time
        className="text-xs text-muted-foreground"
        dateTime={overview.analyzed_at}
        title={analyzedAt}
        aria-label={t("system:storageLastAnalyzed", { time: analyzedAt })}
      >
        {t("system:storageLastAnalyzed", { time: formatRelative(overview.analyzed_at) })}
      </time>
    );
  }
  return (
    <p className="text-xs text-muted-foreground" data-testid="storage-analysis-status">
      {overview.analysis.state === "failed"
        ? t("system:storageAnalysisFailed")
        : t("system:storageAnalysisRunning")}
    </p>
  );
}

function AnalysisTotal({
  overview,
  total,
}: {
  overview: StorageOverviewResponse;
  total: ReturnType<typeof storageAnalysisTotal>;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-2 text-xs" data-testid="storage-analysis-total">
      <span className="font-medium">
        {overview.summary === null
          ? t("system:storageTotalCountedSoFar", { size: formatGigabytes(total.bytes) })
          : t("system:storageTotalCounted", { size: formatGigabytes(total.bytes) })}
      </span>
      {total.partial && (
        <Badge variant="outline" data-testid="storage-analysis-total-partial">
          {t("system:storageTotalPartial")}
        </Badge>
      )}
    </div>
  );
}

function StorageOverviewHeader({
  overview,
  displaySummary,
  loading,
  error,
  total,
}: {
  overview: StorageOverviewResponse;
  displaySummary: StorageSummaryPartial;
  loading?: boolean;
  error?: string | null;
  total: ReturnType<typeof storageAnalysisTotal>;
}) {
  const { t } = useTranslation();
  const analysisRunning = overview.analysis.state === "scanning";
  const isAnalysisFailed = overview.analysis.state === "failed";
  const isLoading = (loading ?? false) || analysisRunning;
  const analyzedAt = overview.analyzed_at ? formatDateTime(overview.analyzed_at) : null;
  const overviewError = error ?? overview.analysis.error;
  return (
    <CardHeader>
      <CardTitle className="flex items-center gap-2 text-base">
        <IconChartPie className="size-4" /> {t("system:storageAnalysisTitle")}
        {isLoading && <Spinner className="size-4" data-testid="storage-overview-spinner" />}
        {isAnalysisFailed && <Badge variant="outline">{t("system:storageAnalysisFailed")}</Badge>}
        {displaySummary.docker?.available === false && (
          <Badge variant="outline">{t("system:storageDockerUnavailableBadge")}</Badge>
        )}
      </CardTitle>
      <CardDescription>{t("system:storageAnalysisDescription")}</CardDescription>
      <div className="flex flex-wrap items-center gap-1">
        <AnalysisStatusTime overview={overview} analyzedAt={analyzedAt} />
        <AnalysisTimingDisclosure overview={overview} />
      </div>
      <AnalysisTotal overview={overview} total={total} />
      {overviewError && (
        <p className="break-words text-xs text-destructive" data-testid="storage-overview-error">
          {t("system:storageSectionUnavailable")}: {overviewError}
        </p>
      )}
    </CardHeader>
  );
}

function StorageOverviewResources({
  resources,
  cleanupDisabledReason,
  temporaryArtifactsCleanupDisabledReason,
  onRunGoCache,
  onRunTemporaryArtifacts,
}: {
  resources: StorageResource[];
  cleanupDisabledReason?: string;
  temporaryArtifactsCleanupDisabledReason?: string;
  onRunGoCache: () => void;
  onRunTemporaryArtifacts: () => void;
}) {
  return (
    <CardContent className="min-w-0">
      <Accordion type="multiple" className="min-w-0">
        {resources.map((resource) => (
          <ResourceRow
            key={resource.id}
            resource={resource}
            goCacheCleanupDisabledReason={cleanupDisabledReason}
            onRunGoCache={onRunGoCache}
            temporaryArtifactsCleanupDisabledReason={temporaryArtifactsCleanupDisabledReason}
            onRunTemporaryArtifacts={onRunTemporaryArtifacts}
          />
        ))}
      </Accordion>
    </CardContent>
  );
}

export function StorageOverviewCard({
  overview,
  settings,
  loading,
  error,
  disabledReason,
  onRunGoCache,
  onRunTemporaryArtifacts = () => {},
}: Props) {
  const { t } = useTranslation();
  if (!overview) {
    return <EmptyStorageOverview isInitialLoading={loading ?? true} error={error} />;
  }
  const displaySummary: StorageSummaryPartial =
    overview.summary ?? overview.analysis.partial_summary ?? {};
  const cleanupDisabledReason = goCacheDisabledReason(t, overview, disabledReason, settings);
  const temporaryArtifactsCleanupDisabledReason = temporaryArtifactsDisabledReason(
    t,
    overview,
    disabledReason,
  );
  const total = storageAnalysisTotal(displaySummary);
  return (
    <Card className="min-w-0" data-testid="storage-overview-card">
      <StorageOverviewHeader
        overview={overview}
        displaySummary={displaySummary}
        loading={loading}
        error={error}
        total={total}
      />
      <StorageOverviewResources
        resources={storageResources(t, overview)}
        cleanupDisabledReason={cleanupDisabledReason}
        temporaryArtifactsCleanupDisabledReason={temporaryArtifactsCleanupDisabledReason}
        onRunGoCache={onRunGoCache}
        onRunTemporaryArtifacts={onRunTemporaryArtifacts}
      />
    </Card>
  );
}

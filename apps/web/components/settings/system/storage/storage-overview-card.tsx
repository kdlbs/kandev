import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@kandev/ui/accordion";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { IconChartPie, IconTrash } from "@tabler/icons-react";
import type { StorageOverviewResponse, StorageQuarantineSummary } from "@/lib/types/system";
import { StorageActionButton } from "./storage-action-button";
import { formatGigabytes } from "./storage-units";

interface Props {
  overview: StorageOverviewResponse | null;
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

function goCacheDisabledReason(overview: StorageOverviewResponse, pendingReason?: string) {
  if (pendingReason) return pendingReason;
  if (overview.summary.go_cache.owned !== true) {
    return "Only a Kandev-owned Go build cache can be cleaned.";
  }
  if ((overview.summary.go_cache.size_bytes ?? 0) <= overview.settings.go_cache.max_bytes) {
    return "The Go build cache is below its configured size limit.";
  }
  return undefined;
}

function quarantineResource(summary: StorageQuarantineSummary): StorageResource {
  if (summary.available === false) {
    return {
      id: "quarantine",
      label: "Quarantined resources",
      value: "Unavailable",
      detail: "Quarantine usage could not be measured",
      warning: summary.warning,
    };
  }
  return {
    id: "quarantine",
    label: "Quarantined resources",
    value: formatGigabytes(summary.size_bytes),
    detail: `${summary.count} items moved aside for recovery before permanent deletion`,
  };
}

function dockerMeasurement(
  available: boolean,
  value: string,
  detail: string,
): Pick<StorageResource, "value" | "detail"> {
  if (!available) {
    return { value: "Unavailable", detail: "Docker usage could not be measured" };
  }
  return { value, detail };
}

function storageResources(overview: StorageOverviewResponse): StorageResource[] {
  const { summary } = overview;
  const dockerWarning = summary.docker.warnings?.join(" · ");
  return [
    {
      id: "workspaces",
      label: "Task workspaces",
      value: formatGigabytes(summary.workspaces.candidate_bytes ?? 0),
      detail: `Active workspaces use ${formatGigabytes(summary.workspaces.active_bytes ?? 0)}`,
      warning: summary.workspaces.warning,
    },
    quarantineResource(summary.quarantine),
    {
      id: "managed-containers",
      label: "Kandev containers",
      ...dockerMeasurement(
        summary.docker.available,
        formatGigabytes(summary.docker.managed_container_bytes ?? 0),
        `${summary.docker.managed_container_count ?? 0} managed containers`,
      ),
      warning: dockerWarning,
    },
    {
      id: "go-cache",
      label: "Go build cache",
      value: formatGigabytes(summary.go_cache.size_bytes ?? 0),
      detail: summary.go_cache.path ?? overview.capabilities.managed_go_cache_path,
      warning: summary.go_cache.warning,
    },
    {
      id: "docker-build-cache",
      label: "Docker build cache",
      ...dockerMeasurement(
        summary.docker.available,
        formatGigabytes(summary.docker.build_cache_bytes),
        overview.capabilities.docker_host || "Default Docker host",
      ),
      warning: dockerWarning,
    },
    {
      id: "docker-unused-images",
      label: "Unused Docker images",
      ...dockerMeasurement(
        summary.docker.available,
        formatGigabytes(summary.docker.unused_image_bytes),
        "Unused by every container and older than the configured age",
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
            <IconTrash className="size-4" /> Clean Go cache
          </StorageActionButton>
        )}
      </AccordionContent>
    </AccordionItem>
  );
}

export function StorageOverviewCard({ overview, disabledReason, onRunGoCache }: Props) {
  if (!overview) {
    return (
      <Card data-testid="storage-overview-card">
        <CardContent className="py-8 text-sm text-muted-foreground">
          Loading storage data…
        </CardContent>
      </Card>
    );
  }
  const { summary } = overview;
  const cleanupDisabledReason = goCacheDisabledReason(overview, disabledReason);
  const resources = storageResources(overview);
  return (
    <Card className="min-w-0" data-testid="storage-overview-card">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <IconChartPie className="size-4" /> Storage analysis
          {!summary.docker.available && <Badge variant="outline">Docker unavailable</Badge>}
        </CardTitle>
        <CardDescription>
          A read-only breakdown of current usage and reclaimable space. Run Analyze occasionally to
          refresh these estimates; it never deletes or moves anything.
        </CardDescription>
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

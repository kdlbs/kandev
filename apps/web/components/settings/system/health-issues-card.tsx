"use client";

import { useRouter } from "@/lib/routing/client-router";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import {
  IconActivity,
  IconAlertTriangle,
  IconCircleCheck,
  IconExternalLink,
  IconInfoCircle,
  IconX,
} from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { useSystemHealth } from "@/hooks/domains/settings/use-system-health";
import type { HealthCheckSummary, HealthIssue, HealthSeverity } from "@/lib/types/health";

function severityIcon(severity: HealthSeverity) {
  if (severity === "error") return <IconAlertTriangle className="h-4 w-4 text-red-500" />;
  if (severity === "warning") return <IconAlertTriangle className="h-4 w-4 text-amber-500" />;
  return <IconInfoCircle className="h-4 w-4 text-blue-500" />;
}

function severityLabel(severity: HealthSeverity): string {
  return severity.charAt(0).toUpperCase() + severity.slice(1);
}

function HealthIssueRow({ issue }: { issue: HealthIssue }) {
  const router = useRouter();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const resolveUrl = (url: string) => url.replace("{workspaceId}", workspaceId ?? "");
  return (
    <div
      className="rounded-md border p-3 space-y-2"
      data-testid={`system-health-issue-${issue.id}`}
    >
      <div className="flex items-start gap-2">
        {severityIcon(issue.severity)}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="font-medium text-sm">{issue.title}</span>
            <Badge variant="outline" className="text-[10px]">
              {severityLabel(issue.severity)}
            </Badge>
          </div>
          {issue.message && <p className="text-xs text-muted-foreground mt-1">{issue.message}</p>}
        </div>
      </div>
      {issue.fix_url && issue.fix_label && (
        <Button
          variant="outline"
          size="sm"
          className="cursor-pointer h-7 text-xs"
          onClick={() => router.push(resolveUrl(issue.fix_url))}
        >
          {issue.fix_label}
          <IconExternalLink className="h-3 w-3 ml-1" />
        </Button>
      )}
    </div>
  );
}

function ChecksPopover({ checks }: { checks: HealthCheckSummary[] }) {
  if (checks.length === 0) return null;
  return (
    <Popover>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-label="What's monitored"
          className="cursor-pointer text-muted-foreground hover:text-foreground transition-colors"
          data-testid="system-health-checks-trigger"
        >
          <IconInfoCircle className="h-4 w-4" />
        </button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72" data-testid="system-health-checks-popover">
        <p className="text-xs font-medium mb-2">System checks</p>
        <ul className="space-y-1.5">
          {checks.map((c) => (
            <li
              key={c.category}
              className="flex items-center gap-2 text-xs"
              data-testid={`system-health-check-${c.category}`}
            >
              {c.passing ? (
                <IconCircleCheck className="h-3.5 w-3.5 text-emerald-500 shrink-0" />
              ) : (
                <IconX className="h-3.5 w-3.5 text-amber-500 shrink-0" />
              )}
              <span>{c.name}</span>
              <span className="ml-auto text-[10px] text-muted-foreground">
                {c.passing ? "Passing" : "Issue"}
              </span>
            </li>
          ))}
        </ul>
      </PopoverContent>
    </Popover>
  );
}

export function HealthIssuesCard() {
  const { issues, checks, loaded } = useSystemHealth();
  const nonInfo = issues.filter((i) => i.severity !== "info");
  const hasIssues = nonInfo.length > 0;

  return (
    <Card data-testid="system-health-card">
      <CardHeader className="flex flex-row items-center justify-between space-y-0">
        <CardTitle className="text-base flex items-center gap-2">
          <IconActivity className="h-4 w-4" />
          Health
          {loaded && (
            <Badge variant={hasIssues ? "destructive" : "secondary"} className="text-[10px]">
              {hasIssues ? `${nonInfo.length} issue${nonInfo.length === 1 ? "" : "s"}` : "Healthy"}
            </Badge>
          )}
        </CardTitle>
        {loaded && <ChecksPopover checks={checks} />}
      </CardHeader>
      <CardContent className="space-y-3">
        {!loaded && (
          <p className="text-xs text-muted-foreground" data-testid="system-health-loading">
            Loading health checks...
          </p>
        )}
        {loaded && issues.length === 0 && (
          <div
            className="flex items-center gap-2 text-sm text-muted-foreground"
            data-testid="system-health-empty"
          >
            <IconCircleCheck className="h-4 w-4 text-emerald-500" />
            All system checks pass.
          </div>
        )}
        {loaded && issues.map((issue) => <HealthIssueRow key={issue.id} issue={issue} />)}
      </CardContent>
    </Card>
  );
}

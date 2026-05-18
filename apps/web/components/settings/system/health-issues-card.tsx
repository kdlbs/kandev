"use client";

import { useRouter } from "next/navigation";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import {
  IconActivity,
  IconAlertTriangle,
  IconCircleCheck,
  IconExternalLink,
  IconInfoCircle,
} from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { useSystemHealth } from "@/hooks/domains/settings/use-system-health";
import type { HealthIssue, HealthSeverity } from "@/lib/types/health";

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

export function HealthIssuesCard() {
  const { issues, loaded } = useSystemHealth();
  const nonInfo = issues.filter((i) => i.severity !== "info");
  const hasIssues = nonInfo.length > 0;

  return (
    <Card data-testid="system-health-card">
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <IconActivity className="h-4 w-4" />
          Health
          {loaded && (
            <Badge variant={hasIssues ? "destructive" : "secondary"} className="text-[10px]">
              {hasIssues ? `${nonInfo.length} issue${nonInfo.length === 1 ? "" : "s"}` : "Healthy"}
            </Badge>
          )}
        </CardTitle>
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

"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useFeature } from "@/hooks/domains/features/use-feature";
import {
  listWorkspaceCoordinatorGrants,
  listWorkspaceCoordinatorAudit,
  revokeCoordinatorGrant,
} from "@/lib/api/domains/coordinator-api";
import type { GrantDTO, AuditEventDTO } from "@/lib/api/domains/coordinator-api";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Skeleton } from "@kandev/ui/skeleton";
import { toast } from "@/lib/toast/sonner";
import { CreateGrantDialog } from "./create-grant-dialog";

type Props = {
  workspaceId: string;
};

function useRevokeGrant(refetch: () => void) {
  const { t } = useTranslation();
  return useCallback(
    async (grant: GrantDTO) => {
      try {
        await revokeCoordinatorGrant(grant.id);
        toast.success(t("workspaces:grantRevoked"));
        refetch();
      } catch {
        toast.error(t("workspaces:grantRevokeFailed"));
      }
    },
    [t, refetch],
  );
}

export function CoordinatorGrantsPage({ workspaceId }: Props) {
  const { t } = useTranslation();
  const flagEnabled = useFeature("coordinatorTaskAuthority");

  const [grants, setGrants] = useState<readonly GrantDTO[]>([]);
  const [audit, setAudit] = useState<readonly AuditEventDTO[]>([]);
  const [grantsLoading, setGrantsLoading] = useState(true);
  const [auditLoading, setAuditLoading] = useState(true);
  const [grantsError, setGrantsError] = useState(false);

  const fetchData = useCallback(async () => {
    if (!flagEnabled) return;
    setGrantsLoading(true);
    setAuditLoading(true);
    setGrantsError(false);
    try {
      const [grantsData, auditData] = await Promise.allSettled([
        listWorkspaceCoordinatorGrants(workspaceId),
        listWorkspaceCoordinatorAudit(workspaceId, { limit: 50 }),
      ]);
      if (grantsData.status === "fulfilled") {
        setGrants(grantsData.value.grants);
      } else {
        setGrantsError(true);
      }
      if (auditData.status === "fulfilled") {
        setAudit(auditData.value.events);
      }
    } finally {
      setGrantsLoading(false);
      setAuditLoading(false);
    }
  }, [workspaceId, flagEnabled]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  if (!flagEnabled) {
    return null;
  }

  const handleRevoke = useRevokeGrant(fetchData);

  return (
    <div className="space-y-6" data-testid="coordinator-grants-page">
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">
          {t("workspaces:coordinatorGrantsDescription")}
        </p>
        <CreateGrantDialog workspaceId={workspaceId} onCreated={fetchData} />
      </div>
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">{t("workspaces:activeGrants")}</CardTitle>
        </CardHeader>
        <CardContent>
          {grantContent(grantsLoading, grantsError, grants, t, handleRevoke)}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle className="text-lg">{t("workspaces:recentAudit")}</CardTitle>
        </CardHeader>
        <CardContent>{auditContent(auditLoading, audit, t)}</CardContent>
      </Card>
    </div>
  );
}

type BadgeKind = "default" | "secondary" | "destructive";

function decisionBadgeVariant(decision: string): BadgeKind {
  if (decision === "allowed") return "default";
  if (decision === "denied") return "destructive";
  return "secondary";
}

function decisionLabel(t: (key: string) => string, decision: string): string {
  if (decision === "allowed") return t("common:allowed");
  if (decision === "denied") return t("common:denied");
  return t("common:pending");
}

function grantContent(
  grantsLoading: boolean,
  grantsError: boolean,
  grants: readonly GrantDTO[],
  t: (key: string) => string,
  onRevoke: (grant: GrantDTO) => void,
): React.ReactNode {
  if (grantsLoading) {
    return (
      <div className="space-y-2">
        <Skeleton className="h-8 w-full" />
        <Skeleton className="h-8 w-full" />
        <Skeleton className="h-8 w-full" />
      </div>
    );
  }
  if (grantsError) {
    return <p className="text-sm text-destructive">{t("common:errorLoading")}</p>;
  }
  if (grants.length === 0) {
    return <p className="text-sm text-muted-foreground">{t("workspaces:noGrants")}</p>;
  }
  return (
    <div className="space-y-3">
      {grants.map((grant) => (
        <GrantRow key={grant.id} grant={grant} onRevoke={onRevoke} />
      ))}
    </div>
  );
}

function auditContent(
  auditLoading: boolean,
  audit: readonly AuditEventDTO[],
  t: (key: string) => string,
): React.ReactNode {
  if (auditLoading) {
    return (
      <div className="space-y-2">
        <Skeleton className="h-8 w-full" />
        <Skeleton className="h-8 w-full" />
      </div>
    );
  }
  if (audit.length === 0) {
    return <p className="text-sm text-muted-foreground">{t("workspaces:noAuditEvents")}</p>;
  }
  return (
    <div className="space-y-2">
      {audit.map((event) => (
        <AuditRow key={event.id} event={event} />
      ))}
    </div>
  );
}

function GrantRow({ grant, onRevoke }: { grant: GrantDTO; onRevoke: (grant: GrantDTO) => void }) {
  const { t } = useTranslation();
  const isRevoked = !!grant.revoked_at;
  const capabilities = grant.capabilities
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);

  return (
    <div
      className={`flex items-center justify-between rounded-lg border p-3 ${
        isRevoked ? "opacity-50" : ""
      }`}
      data-testid={`grant-row-${grant.id}`}
    >
      <div className="min-w-0 space-y-1">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium truncate">
            {t("workspaces:grantForTask")}: {grant.coordinator_task_id}
          </span>
          {isRevoked ? (
            <Badge variant="secondary">{t("workspaces:revoked")}</Badge>
          ) : (
            <Badge variant="default">{t("workspaces:active")}</Badge>
          )}
        </div>
        <div className="flex flex-wrap gap-1">
          <Badge variant="outline" className="text-xs">
            {t("workspaces:scope")}: {grant.scope_kind}
            {grant.scope_id ? ` / ${grant.scope_id.slice(0, 8)}…` : ""}
          </Badge>
          {capabilities.map((cap) => (
            <Badge key={cap} variant="secondary" className="text-xs">
              {cap}
            </Badge>
          ))}
        </div>
        {grant.note && <p className="text-xs text-muted-foreground truncate">{grant.note}</p>}
        <p className="text-xs text-muted-foreground">
          {t("workspaces:grantedBy")}: {grant.granted_by_user_id} &middot;{" "}
          {new Date(grant.granted_at).toLocaleString()}
        </p>
      </div>
      {!isRevoked && (
        <Button
          variant="destructive"
          size="sm"
          onClick={() => onRevoke(grant)}
          data-testid={`revoke-grant-${grant.id}`}
        >
          {t("common:revoke")}
        </Button>
      )}
    </div>
  );
}

function AuditRow({ event }: { event: AuditEventDTO }) {
  const { t } = useTranslation();
  const badgeVariant = decisionBadgeVariant(event.decision);
  const labelText = decisionLabel(t, event.decision);

  return (
    <div
      className="flex items-center justify-between rounded-md border p-2 text-sm"
      data-testid={`audit-row-${event.id}`}
    >
      <div className="min-w-0 space-y-1">
        <div className="flex items-center gap-2">
          <span className="font-medium">{event.action}</span>
          <Badge variant={badgeVariant} className="text-xs">
            {labelText}
          </Badge>
        </div>
        <p className="text-xs text-muted-foreground">
          {t("workspaces:actorTask")}: {event.actor_task_id.slice(0, 8)}… &middot;
          {t("workspaces:targetTask")}: {event.target_task_id.slice(0, 8)}… &middot;
          {t("workspaces:capability")}: {event.capability}
        </p>
        {event.detail && <p className="text-xs text-muted-foreground">{event.detail}</p>}
      </div>
      <span className="text-xs text-muted-foreground shrink-0">
        {new Date(event.occurred_at).toLocaleString()}
      </span>
    </div>
  );
}

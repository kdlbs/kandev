"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconBuilding, IconPlayerPause, IconPlayerPlay, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { useOrganizations } from "@/hooks/domains/org/use-organizations";
import type { Org } from "@/lib/types/org";
import { CreateOrgCard } from "./create-org-card";
import { DeleteOrgDialog } from "./delete-org-dialog";
import { OrgAdminDialog } from "./org-admin-dialog";

type OrgRowProps = {
  org: Org;
  busy: boolean;
  onSuspend: (id: string) => void;
  onResume: (id: string) => void;
  onDelete: (org: Org) => void;
  onAddAdmin: (org: Org) => void;
};

function OrgRow({ org, busy, onSuspend, onResume, onDelete, onAddAdmin }: OrgRowProps) {
  const { t } = useTranslation();
  const suspended = org.status === "suspended";
  return (
    <Card data-testid="organization-row">
      <CardContent className="flex flex-wrap items-center gap-3 py-4">
        <IconBuilding className="text-muted-foreground size-5" aria-hidden />
        <div className="min-w-0 flex-1">
          <p className="truncate font-medium">{org.name}</p>
          <p className="text-muted-foreground truncate font-mono text-xs">{org.slug}</p>
        </div>
        {org.is_default ? <Badge variant="outline">{t("orgs:defaultBadge")}</Badge> : null}
        <Badge variant={suspended ? "destructive" : "secondary"}>
          {suspended ? t("orgs:statusSuspended") : t("orgs:statusActive")}
        </Badge>
        <Button
          variant="ghost"
          size="sm"
          className="cursor-pointer"
          disabled={busy}
          onClick={() => onAddAdmin(org)}
        >
          {t("orgs:addAdmin")}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          className="cursor-pointer"
          disabled={busy}
          onClick={() => (suspended ? onResume(org.id) : onSuspend(org.id))}
        >
          {suspended ? (
            <IconPlayerPlay className="size-4" aria-hidden />
          ) : (
            <IconPlayerPause className="size-4" aria-hidden />
          )}
          {suspended ? t("orgs:resume") : t("orgs:suspend")}
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="cursor-pointer"
          disabled={busy || org.is_default}
          aria-label={t("orgs:delete")}
          onClick={() => onDelete(org)}
        >
          <IconTrash className="size-4" aria-hidden />
        </Button>
      </CardContent>
    </Card>
  );
}

/**
 * The instance operator's organization list.
 *
 * Operator is an administration tier, not a visibility one: nothing on this
 * page reads any organization's workspaces, tasks or secrets.
 */
export function OrganizationsPage() {
  const { t } = useTranslation();
  const orgs = useOrganizations();
  const [deleting, setDeleting] = useState<Org | null>(null);
  const [adminFor, setAdminFor] = useState<Org | null>(null);

  if (orgs.isOperator === null) return null;
  if (!orgs.isOperator) {
    return (
      <Card>
        <CardContent className="text-muted-foreground py-10 text-center text-sm">
          {t("orgs:operatorOnly")}
        </CardContent>
      </Card>
    );
  }

  return (
    // The route shell already renders the page title and description, so this
    // page starts at its content.
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>{t("orgs:boundaryTitle")}</CardTitle>
          <CardDescription>{t("orgs:boundaryDescription")}</CardDescription>
        </CardHeader>
      </Card>

      <CreateOrgCard busy={orgs.busy} onCreate={orgs.create} />

      <div className="grid gap-3">
        {orgs.orgs.map((org) => (
          <OrgRow
            key={org.id}
            org={org}
            busy={orgs.busy}
            onSuspend={orgs.suspend}
            onResume={orgs.resume}
            onDelete={setDeleting}
            onAddAdmin={setAdminFor}
          />
        ))}
      </div>

      <DeleteOrgDialog
        org={deleting}
        busy={orgs.busy}
        onClose={() => setDeleting(null)}
        onConfirm={(org) => orgs.remove(org.id, org.slug, () => setDeleting(null))}
      />
      <OrgAdminDialog
        org={adminFor}
        busy={orgs.busy}
        onClose={() => setAdminFor(null)}
        onCreate={(org, admin) => orgs.addAdmin(org.id, admin, () => setAdminFor(null))}
      />
    </div>
  );
}

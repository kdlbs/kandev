"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconSitemap } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Combobox } from "@/components/combobox";
import { toast } from "@/lib/toast/sonner";
import { useOrgUnits } from "@/hooks/domains/org/use-org-units";
import { placeWorkspace } from "@/lib/api/domains/org-units-api";
import { hasScope, SCOPE } from "@/lib/types/team-access";

type Props = {
  workspaceId: string;
  unitId?: string;
  scopes?: readonly string[];
};

/**
 * Where a workspace sits, and therefore who reaches it.
 *
 * Moving it is the only way to widen or narrow access: roles combine by
 * maximum, so nothing can be subtracted from a workspace in place. That makes
 * this control the counterpart of unit membership rather than a filing
 * convenience.
 */
export function WorkspacePlacementCard({ workspaceId, unitId, scopes }: Props) {
  const { t } = useTranslation();
  const units = useOrgUnits();
  const canManage = hasScope(scopes, SCOPE.workspaceManage);
  const [saving, setSaving] = useState(false);
  const [current, setCurrent] = useState(unitId ?? "");

  const move = async (next: string) => {
    if (!next || next === current) return;
    const previous = current;
    setCurrent(next);
    setSaving(true);
    try {
      await placeWorkspace(workspaceId, next);
      toast.success(t("workspaces:placement.moved"));
    } catch (err) {
      setCurrent(previous);
      toast.error(err instanceof Error ? err.message : t("workspaces:placement.failed"));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card data-testid="workspace-placement-card">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <IconSitemap className="text-muted-foreground size-4" aria-hidden />
          {t("workspaces:placement.title")}
        </CardTitle>
        <CardDescription>{t("workspaces:placement.description")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-wrap items-center gap-2">
        <Combobox
          options={units.units.map((unit) => ({
            value: unit.id,
            label: `${"  ".repeat(unit.depth)}${unit.name}`,
            keywords: [unit.name],
          }))}
          value={current}
          onValueChange={(next) => void move(next)}
          disabled={!canManage || saving || units.loading}
          placeholder={t("workspaces:placement.pick")}
          searchPlaceholder={t("workspaces:placement.search")}
          emptyMessage={t("workspaces:placement.none")}
          triggerClassName="h-9 w-full max-w-sm justify-between"
          testId="workspace-placement-picker"
        />
        {!canManage && (
          <Button variant="ghost" size="sm" disabled>
            {t("workspaces:placement.ownerOnly")}
          </Button>
        )}
      </CardContent>
    </Card>
  );
}

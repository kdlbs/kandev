"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconFolder, IconFolderPlus, IconTrash, IconUser, IconUsers } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { useOrgUnits, type UnitRow } from "@/hooks/domains/org/use-org-units";
import type { OrgUnit } from "@/lib/api/domains/org-units-api";
import { UnitMembersDialog } from "./unit-members-dialog";

function UnitIcon({ kind }: { kind: OrgUnit["kind"] }) {
  if (kind === "personal") return <IconUser className="text-muted-foreground size-4" aria-hidden />;
  return <IconFolder className="text-muted-foreground size-4" aria-hidden />;
}

type RowProps = {
  unit: UnitRow;
  busy: boolean;
  onAddChild: (unit: UnitRow) => void;
  onManageMembers: (unit: UnitRow) => void;
  onDelete: (unit: UnitRow) => void;
};

function UnitTreeRow({ unit, busy, onAddChild, onManageMembers, onDelete }: RowProps) {
  const { t } = useTranslation();
  // A personal unit takes no members and cannot be moved or deleted, so it
  // shows its owner rather than controls that would always be refused.
  const personal = unit.kind === "personal";
  const root = unit.kind === "root";
  return (
    <div
      data-testid="unit-row"
      className="flex flex-wrap items-center gap-2 rounded-md border px-3 py-2"
      style={{ marginInlineStart: `${unit.depth * 20}px` }}
    >
      <UnitIcon kind={unit.kind} />
      <span className="truncate text-sm font-medium">{unit.name}</span>
      {root && <Badge variant="secondary">{t("settings:unitRootBadge")}</Badge>}
      {personal && <Badge variant="outline">{t("settings:unitPersonalBadge")}</Badge>}
      <span className="ml-auto flex items-center gap-1">
        {!personal && (
          <Button
            size="sm"
            variant="ghost"
            disabled={busy}
            data-testid="unit-add-child"
            onClick={() => onAddChild(unit)}
          >
            <IconFolderPlus className="size-4" /> {t("settings:unitAddChild")}
          </Button>
        )}
        {!personal && (
          <Button
            size="sm"
            variant="ghost"
            disabled={busy}
            data-testid="unit-members"
            onClick={() => onManageMembers(unit)}
          >
            <IconUsers className="size-4" /> {t("settings:unitMembers")}
          </Button>
        )}
        {!personal && !root && (
          <Button
            size="icon"
            variant="ghost"
            disabled={busy}
            aria-label={t("settings:unitDelete")}
            data-testid="unit-delete"
            onClick={() => onDelete(unit)}
          >
            <IconTrash className="size-4" />
          </Button>
        )}
      </span>
    </div>
  );
}

/**
 * The organization tree.
 *
 * A unit holds child units, workspaces and people, and membership reaches
 * everything beneath it. Departments and teams are the same thing at different
 * depths, which is why there is one control here rather than one per level.
 */
export function UnitsPage() {
  const { t } = useTranslation();
  const units = useOrgUnits();
  const [parentForNew, setParentForNew] = useState<UnitRow | null>(null);
  const [newName, setNewName] = useState("");
  const [membersFor, setMembersFor] = useState<UnitRow | null>(null);

  const submitNew = async () => {
    if (!parentForNew || !newName.trim()) return;
    if (await units.create(parentForNew.id, newName.trim())) {
      setNewName("");
      setParentForNew(null);
    }
  };

  return (
    <div className="flex flex-col gap-4" data-testid="units-page">
      <Card>
        <CardHeader>
          <CardTitle>{t("settings:unitsTitle")}</CardTitle>
          <CardDescription>{t("settings:unitsDescription")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          {units.loading && <p className="text-muted-foreground text-sm">{t("common:loading")}</p>}
          {!units.loading && units.units.length === 0 && (
            <p className="text-muted-foreground text-sm">{t("settings:unitsEmpty")}</p>
          )}
          {units.units.map((unit) => (
            <UnitTreeRow
              key={unit.id}
              unit={unit}
              busy={units.busy}
              onAddChild={setParentForNew}
              onManageMembers={setMembersFor}
              onDelete={(u) => void units.remove(u.id)}
            />
          ))}
        </CardContent>
      </Card>

      {parentForNew && (
        <Card data-testid="unit-create-card">
          <CardHeader>
            <CardTitle>{t("settings:unitCreateTitle", { parent: parentForNew.name })}</CardTitle>
            <CardDescription>{t("settings:unitCreateDescription")}</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-wrap items-center gap-2">
            <Input
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              placeholder={t("settings:unitNamePlaceholder")}
              className="max-w-64"
              data-testid="unit-name-input"
            />
            <Button disabled={units.busy || !newName.trim()} onClick={() => void submitNew()}>
              {t("settings:unitCreate")}
            </Button>
            <Button variant="ghost" onClick={() => setParentForNew(null)}>
              {t("common:cancel")}
            </Button>
          </CardContent>
        </Card>
      )}

      <UnitMembersDialog
        unit={membersFor}
        busy={units.busy}
        onClose={() => setMembersFor(null)}
        onAdd={units.addMember}
        onRemove={units.removeMember}
      />
    </div>
  );
}

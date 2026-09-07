"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Combobox } from "@/components/combobox";
import { useAssignablePeople } from "@/hooks/domains/users/use-assignable-people";
import { listUnitMembers, type OrgUnit, type UnitMember } from "@/lib/api/domains/org-units-api";
import { ASSIGNABLE_WORKSPACE_ROLES } from "@/lib/types/team-access";

type Props = {
  unit: OrgUnit | null;
  busy: boolean;
  onClose: () => void;
  onAdd: (unitId: string, userId: string, role: string) => Promise<boolean>;
  onRemove: (unitId: string, userId: string) => Promise<boolean>;
};

/**
 * Who is in a unit, and therefore who reaches every workspace beneath it.
 *
 * Membership is recorded once here rather than per workspace, which is the
 * whole point of the tree: a board created next week is reached by the same
 * people with no further bookkeeping.
 */
export function UnitMembersDialog({ unit, busy, onClose, onAdd, onRemove }: Props) {
  const { t } = useTranslation();
  const { people, nameFor } = useAssignablePeople(undefined, { enabled: Boolean(unit) });
  const [members, setMembers] = useState<UnitMember[]>([]);
  const [userId, setUserId] = useState("");
  const [role, setRole] = useState<string>(ASSIGNABLE_WORKSPACE_ROLES[0]);

  useEffect(() => {
    if (!unit) return;
    let cancelled = false;
    listUnitMembers(unit.id)
      .then((res) => {
        if (!cancelled) setMembers(res.members ?? []);
      })
      .catch(() => {
        if (!cancelled) setMembers([]);
      });
    return () => {
      cancelled = true;
    };
  }, [unit, busy]);

  if (!unit) return null;

  const add = async () => {
    if (!userId) return;
    if (await onAdd(unit.id, userId, role)) setUserId("");
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent data-testid="unit-members-dialog">
        <DialogHeader>
          <DialogTitle>{t("settings:unitMembersTitle", { unit: unit.name })}</DialogTitle>
          <DialogDescription>{t("settings:unitMembersDescription")}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-2">
          {members.length === 0 && (
            <p className="text-muted-foreground text-sm">{t("settings:unitNoMembers")}</p>
          )}
          {members.map((member) => (
            <div
              key={member.user_id}
              data-testid="unit-member-row"
              className="flex items-center justify-between gap-2 rounded-md border px-3 py-2"
            >
              <span className="truncate text-sm">{nameFor(member.user_id)}</span>
              <span className="flex items-center gap-2">
                <span className="text-muted-foreground text-xs">{member.role}</span>
                <Button
                  size="icon"
                  variant="ghost"
                  disabled={busy}
                  aria-label={t("settings:unitRemoveMember")}
                  onClick={() => void onRemove(unit.id, member.user_id)}
                >
                  <IconTrash className="size-4" />
                </Button>
              </span>
            </div>
          ))}
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Combobox
            options={people.map((p) => ({ value: p.id, label: p.name }))}
            value={userId}
            onValueChange={setUserId}
            placeholder={t("settings:unitPickPerson")}
            searchPlaceholder={t("task:searchPeople")}
            emptyMessage={t("task:noPeopleFound")}
            triggerClassName="h-8 min-w-48"
            testId="unit-member-picker"
          />
          <Combobox
            options={ASSIGNABLE_WORKSPACE_ROLES.map((r) => ({ value: r, label: r }))}
            value={role}
            onValueChange={setRole}
            showSearch={false}
            triggerClassName="h-8 w-40"
            testId="unit-role-picker"
          />
          <Button size="sm" disabled={busy || !userId} onClick={() => void add()}>
            {t("settings:unitAddMember")}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

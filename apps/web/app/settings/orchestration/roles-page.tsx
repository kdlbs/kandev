import { PersonaIconField } from "./persona-icon-field";
import { useAppStore } from "@/components/state-provider";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { Label } from "@kandev/ui/label";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { toast } from "@/lib/toast/sonner";
import {
  useOrchestrationData,
  notifyOrchestrationChanged,
} from "@/hooks/domains/orchestration/use-orchestration-data";
import {
  listOrchestratorRoles,
  saveOrchestratorRole,
  deleteOrchestratorRole,
  type OrchestratorRole,
} from "@/lib/api/domains/orchestration-api";
import { OrchestrationGate } from "./orchestration-gate";
export function OrchestrationRolesPage() {
  return (
    <OrchestrationGate>
      <RoleList />
    </OrchestrationGate>
  );
}
function RoleList() {
  const { t } = useTranslation();
  const userRole = useAppStore((s) => s.auth.user?.role);
  const canEdit = userRole === undefined || userRole === "admin";
  const { data, error } = useOrchestrationData(listOrchestratorRoles);
  const [busy, setBusy] = useState(false);
  const create = async () => {
    setBusy(true);
    try {
      await saveOrchestratorRole({
        name: t("orchestration:newOrchestratorRole"),
        instructions: "",
      });
      notifyOrchestrationChanged();
    } catch (e) {
      toast.error(String(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <section className="max-w-3xl space-y-5" data-testid="orchestration-roles">
      <h2 className="text-2xl font-bold">{t("orchestration:orchestration")}</h2>
      <p className="text-sm text-muted-foreground">{t("orchestration:rolesHint")}</p>
      <Link className="block underline" href="/settings/workspaces">
        {t("common:workspaces")}
      </Link>
      <Button onClick={create} disabled={busy || !canEdit}>
        {t("orchestration:addOrchestratorRole")}
      </Button>
      {error && <p role="alert">{error}</p>}
      {!data && !error && <p>{t("common:loading")}</p>}
      {data?.roles.map((role) => (
        <RoleEditor key={role.id} initial={role} />
      ))}
    </section>
  );
}
function RoleEditor({ initial }: { initial: OrchestratorRole }) {
  const { t } = useTranslation();
  const userRole = useAppStore((s) => s.auth.user?.role);
  const canEdit = userRole === undefined || userRole === "admin";
  const [value, setValue] = useState(initial);
  const [saved, setSaved] = useState(initial);
  useSettingsSaveContributor({
    id: `orchestrator-role-${initial.id}`,
    revision: JSON.stringify(value),
    isDirty: JSON.stringify(value) !== JSON.stringify(saved),
    canSave: canEdit && !!value.name.trim(),
    save: async () => {
      await saveOrchestratorRole(value);
      setSaved(value);
      notifyOrchestrationChanged();
    },
    discard: () => setValue(saved),
  });
  const remove = async () => {
    try {
      await deleteOrchestratorRole(initial.id);
      notifyOrchestrationChanged();
    } catch (e) {
      toast.error(String(e));
    }
  };
  return (
    <article
      className="rounded-lg border p-4 space-y-3"
      data-testid="orchestrator-role-editor"
      data-role-id={initial.id}
    >
      <PersonaIconField
        name={value.name}
        icon={value.icon}
        disabled={!canEdit}
        onChange={(icon) => setValue({ ...value, icon })}
      />
      <Label htmlFor={`role-name-${initial.id}`}>{t("orchestration:name")}</Label>
      <Input
        id={`role-name-${initial.id}`}
        disabled={!canEdit}
        maxLength={100}
        value={value.name}
        onChange={(e) => setValue({ ...value, name: e.target.value })}
      />
      <Label htmlFor={`role-instructions-${initial.id}`}>{t("orchestration:instructions")}</Label>
      <Textarea
        id={`role-instructions-${initial.id}`}
        disabled={!canEdit}
        maxLength={32000}
        value={value.instructions}
        onChange={(e) => setValue({ ...value, instructions: e.target.value })}
      />
      <Button
        variant="outline"
        onClick={remove}
        disabled={!canEdit || initial.id === "chief-of-staff"}
      >
        {t("orchestration:delete")}
      </Button>
    </article>
  );
}

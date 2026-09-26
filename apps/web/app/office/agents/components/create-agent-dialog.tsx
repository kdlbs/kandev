"use client";

import { useState, useCallback } from "react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@kandev/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { toast } from "@/lib/toast/sonner";
import { useAppStore } from "@/components/state-provider";
import { selectOfficeAgentProfiles } from "@/lib/state/slices/office/selectors";
import { createAgentProfile } from "@/lib/api/domains/office-api";
import type { AgentRole, AgentProfile } from "@/lib/state/slices/office/types";
import { useTranslation } from "react-i18next";

type CreateAgentDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

type FormState = {
  name: string;
  role: AgentRole;
  reportsTo: string;
  budgetCents: number;
  maxConcurrent: number;
  executorPref: string;
};

const INITIAL_STATE: FormState = {
  name: "",
  role: "worker",
  reportsTo: "",
  budgetCents: 0,
  maxConcurrent: 1,
  executorPref: "",
};

function NameField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const { t } = useTranslation();
  return (
    <div>
      <Label>{t("office:name")}</Label>
      <Input
        placeholder={t("office:eGFrontendWorker")}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="mt-1"
        autoFocus
      />
      <p className="text-xs text-muted-foreground mt-1">{t("office:aUniqueNameForThisAgent")}</p>
    </div>
  );
}

function RoleAndReports({
  role,
  reportsTo,
  agents,
  roles,
  onChange,
}: {
  role: AgentRole;
  reportsTo: string;
  agents: AgentProfile[];
  roles: Array<{ id: string; label: string }>;
  onChange: (patch: Partial<FormState>) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex gap-4">
      <div className="flex-1">
        <Label>{t("office:role")}</Label>
        <Select value={role} onValueChange={(v) => onChange({ role: v as AgentRole })}>
          <SelectTrigger className="mt-1 cursor-pointer">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {roles.map((r) => (
              <SelectItem key={r.id} value={r.id} className="cursor-pointer">
                {r.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground mt-1">
          {t("office:ceoManagesOtherAgentsWorkersExecute")}
        </p>
      </div>
      <div className="flex-1">
        <Label>{t("office:reportsTo")}</Label>
        <Select
          value={reportsTo || "__none__"}
          onValueChange={(v) => onChange({ reportsTo: v === "__none__" ? "" : v })}
        >
          <SelectTrigger className="mt-1 cursor-pointer">
            <SelectValue placeholder={t("office:noneTopLevel")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__none__" className="cursor-pointer">
              {t("office:none")}
            </SelectItem>
            {agents.map((a) => (
              <SelectItem key={a.id} value={a.id} className="cursor-pointer">
                {a.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground mt-1">{t("office:whichAgentManagesThisOne")}</p>
      </div>
    </div>
  );
}

function BudgetAndConcurrency({
  budgetCents,
  maxConcurrent,
  onChange,
}: {
  budgetCents: number;
  maxConcurrent: number;
  onChange: (patch: Partial<FormState>) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex gap-4">
      <div className="flex-1">
        <Label>{t("office:monthlyBudget")}</Label>
        <Input
          type="number"
          min={0}
          value={budgetCents / 100}
          onChange={(e) => onChange({ budgetCents: Math.round(Number(e.target.value) * 100) })}
          className="mt-1"
        />
        <p className="text-xs text-muted-foreground mt-1">
          {t("office:monthlySpendingLimit0Unlimited")}
        </p>
      </div>
      <div className="flex-1">
        <Label>{t("office:maxConcurrent")}</Label>
        <Input
          type="number"
          min={1}
          max={10}
          value={maxConcurrent}
          onChange={(e) => onChange({ maxConcurrent: Number(e.target.value) })}
          className="mt-1"
        />
        <p className="text-xs text-muted-foreground mt-1">{t("office:howManyTasksThisAgentCan")}</p>
      </div>
    </div>
  );
}

function ExecutorPreferenceField({
  value,
  executorTypes,
  onChange,
}: {
  value: string;
  executorTypes: Array<{ id: string; label: string }>;
  onChange: (v: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div>
      <Label>{t("office:executorPreference")}</Label>
      <Select
        value={value || "__inherit__"}
        onValueChange={(v) => onChange(v === "__inherit__" ? "" : v)}
      >
        <SelectTrigger className="mt-1 cursor-pointer">
          <SelectValue placeholder={t("office:inheritFromProjectWorkspace")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="__inherit__" className="cursor-pointer">
            {t("office:inherit")}
          </SelectItem>
          {executorTypes.map((et) => (
            <SelectItem key={et.id} value={et.id} className="cursor-pointer">
              {et.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground mt-1">
        {t("office:howAgentSessionsRunInheritProject")}
      </p>
    </div>
  );
}

// Catalog keys, not copy — module scope freezes a `t()` at the boot locale. The
// `id`s are the wire role / executor-type values. `CEO`, `QA` and `DevOps`
// resolve to themselves; they are kept as keys so a locale that spells them
// differently still can.
const FALLBACK_ROLES = [
  { id: "ceo", labelKey: "office:roleCeo" },
  { id: "worker", labelKey: "office:roleWorker" },
  { id: "specialist", labelKey: "office:roleSpecialist" },
  { id: "assistant", labelKey: "office:roleAssistant" },
  { id: "security", labelKey: "office:roleSecurity" },
  { id: "qa", labelKey: "office:roleQa" },
  { id: "devops", labelKey: "office:roleDevops" },
];

const FALLBACK_EXECUTOR_TYPES = [
  { id: "local_pc", labelKey: "office:localStandalone" },
  { id: "local_docker", labelKey: "office:localDocker" },
  { id: "sprites", labelKey: "office:spritesRemoteSandbox" },
];

export function CreateAgentDialog({ open, onOpenChange }: CreateAgentDialogProps) {
  const { t } = useTranslation();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const agents = useAppStore(selectOfficeAgentProfiles);
  const meta = useAppStore((s) => s.office.meta);
  const addOfficeAgentProfile = useAppStore((s) => s.addOfficeAgentProfile);

  const roles =
    meta?.roles.map((r) => ({ id: r.id, label: r.label })) ??
    FALLBACK_ROLES.map((r) => ({ id: r.id, label: t(r.labelKey) }));
  const executorTypes =
    meta?.executorTypes.map((e) => ({ id: e.id, label: e.label })) ??
    FALLBACK_EXECUTOR_TYPES.map((e) => ({ id: e.id, label: t(e.labelKey) }));

  const [state, setState] = useState<FormState>(INITIAL_STATE);
  const [submitting, setSubmitting] = useState(false);

  const handleChange = useCallback(
    (patch: Partial<FormState>) => setState((prev) => ({ ...prev, ...patch })),
    [],
  );

  const handleCreate = useCallback(async () => {
    if (!state.name.trim() || !workspaceId) return;
    setSubmitting(true);
    try {
      const result = await createAgentProfile(workspaceId, {
        name: state.name.trim(),
        role: state.role,
        reportsTo: state.reportsTo || undefined,
        budgetMonthlyCents: state.budgetCents,
        maxConcurrentSessions: state.maxConcurrent,
        executorPreference: state.executorPref ? { type: state.executorPref } : undefined,
      } as Partial<AgentProfile>);
      if (result) {
        addOfficeAgentProfile(workspaceId, result);
      }
      setState(INITIAL_STATE);
      onOpenChange(false);
      toast.success(
        result?.status === "pending_approval"
          ? t("office:agentAwaitingApproval")
          : t("office:agentCreated"),
      );
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("office:failedToCreateAgent"));
    } finally {
      setSubmitting(false);
    }
  }, [state, workspaceId, addOfficeAgentProfile, onOpenChange]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("office:createAgent")}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <NameField value={state.name} onChange={(v) => handleChange({ name: v })} />
          <RoleAndReports
            role={state.role}
            reportsTo={state.reportsTo}
            agents={agents}
            roles={roles}
            onChange={handleChange}
          />
          <BudgetAndConcurrency
            budgetCents={state.budgetCents}
            maxConcurrent={state.maxConcurrent}
            onChange={handleChange}
          />
          <ExecutorPreferenceField
            value={state.executorPref}
            executorTypes={executorTypes}
            onChange={(v) => handleChange({ executorPref: v })}
          />
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)} className="cursor-pointer">
            {t("common:cancel")}
          </Button>
          <Button
            onClick={handleCreate}
            disabled={!state.name.trim() || submitting}
            className="cursor-pointer"
          >
            {submitting ? t("office:creating") : t("office:createAgent")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

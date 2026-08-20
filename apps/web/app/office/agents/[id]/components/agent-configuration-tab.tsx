"use client";

import { useState, useCallback, useEffect, useMemo, useRef } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { toast } from "@/lib/toast/sonner";
import { useAppStore } from "@/components/state-provider";
import { selectOfficeAgentProfiles } from "@/lib/state/slices/office/selectors";
import { updateAgentProfile } from "@/lib/api/domains/office-api";
import { ApiError } from "@/lib/api/client";
import type { AgentProfile, AgentRole } from "@/lib/state/slices/office/types";
import { AgentRoutingCard } from "./agent-routing-card";
import { reportsToOptions } from "./reports-to-options";
import { useTranslation } from "react-i18next";

type AgentConfigurationTabProps = {
  agent: AgentProfile;
};

function agentValidationMessage(error: unknown, translate: (key: string) => string): string | null {
  if (!(error instanceof ApiError) || !error.body || typeof error.body !== "object") return null;
  const code = "code" in error.body ? error.body.code : null;
  if (typeof code !== "string") return null;
  // i18n-exempt: backend validation codes, not user-facing copy.
  switch (code) {
    case "agent_ceo_reports_to":
      return translate("office:agentCeoReportsToError");
    case "agent_reports_to_invalid":
      return translate("office:agentReportsToInvalidError");
    case "agent_reports_to_self":
      return translate("office:agentReportsToSelfError");
    case "agent_reports_to_cycle":
      return translate("office:agentReportsToCycleError");
    default:
      return null;
  }
}

// Catalog keys, not copy — module scope freezes a `t()` at the boot locale.
// The record keys are wire values and stay untranslated.
const FALLBACK_ROLES: Array<{ id: string; labelKey: string }> = [
  { id: "ceo", labelKey: "office:roleCeo" },
  { id: "worker", labelKey: "office:roleWorker" },
  { id: "specialist", labelKey: "office:roleSpecialist" },
  { id: "assistant", labelKey: "office:roleAssistant" },
  { id: "security", labelKey: "office:roleSecurity" },
  { id: "qa", labelKey: "office:roleQa" },
  { id: "devops", labelKey: "office:roleDevops" },
];

const FALLBACK_EXECUTOR_TYPES: Array<{ id: string; labelKey: string }> = [
  { id: "local_pc", labelKey: "office:localStandalone" },
  { id: "local_docker", labelKey: "office:localDocker" },
  { id: "sprites", labelKey: "office:spritesRemoteSandbox" },
];

const CAPABILITY_LABEL_KEYS: Record<string, string> = {
  post_comment: "office:capabilityPostComment",
  update_task_status: "office:capabilityUpdateTaskStatus",
  create_subtask: "office:capabilityCreateSubtask",
  create_agent: "office:capabilityCreateAgent",
  request_approval: "office:capabilityRequestApproval",
  read_memory: "office:capabilityReadMemory",
  write_memory: "office:capabilityWriteMemory",
  list_skills: "office:capabilityListSkills",
  spawn_agent_run: "office:capabilitySpawnAgentRun",
  modify_agents: "office:capabilityModifyAgents",
  delete_skills: "office:capabilityDeleteSkills",
};

type FormState = {
  name: string;
  role: AgentRole;
  reportsTo: string;
  budgetMonthlyCents: number;
  maxConcurrentSessions: number;
  executorType: string;
};

type FormField = keyof FormState;

const FORM_FIELDS: FormField[] = [
  "name",
  "role",
  "reportsTo",
  "budgetMonthlyCents",
  "maxConcurrentSessions",
  "executorType",
];

function initialForm(agent: AgentProfile): FormState {
  return {
    name: agent.name,
    role: agent.role,
    reportsTo: agent.role === "ceo" ? "" : (agent.reportsTo ?? ""),
    budgetMonthlyCents: agent.budgetMonthlyCents,
    maxConcurrentSessions: agent.maxConcurrentSessions,
    executorType: agent.executorPreference?.type ?? "",
  };
}

export function AgentConfigurationTab({ agent }: AgentConfigurationTabProps) {
  const { t } = useTranslation();
  const meta = useAppStore((s) => s.office.meta);
  const allOfficeAgents = useAppStore(selectOfficeAgentProfiles);

  const roles =
    meta?.roles.map((r) => ({ id: r.id, label: r.label })) ??
    FALLBACK_ROLES.map((r) => ({ id: r.id, label: t(r.labelKey) }));
  const executorTypes =
    meta?.executorTypes.map((e) => ({ id: e.id, label: e.label })) ??
    FALLBACK_EXECUTOR_TYPES.map((e) => ({ id: e.id, label: t(e.labelKey) }));

  const { form, saving, dirty, patch, handleSave } = useAgentConfigurationForm(agent);

  const reportsToChoices = useMemo(
    () => (form.role === "ceo" ? [] : reportsToOptions(allOfficeAgents, agent.id)),
    [allOfficeAgents, agent.id, form.role],
  );

  return (
    <div className="space-y-4 mt-4" data-testid="agent-configuration-tab">
      <IdentityCard
        name={form.name}
        role={form.role}
        roles={roles}
        reportsTo={form.reportsTo}
        reportsToChoices={reportsToChoices}
        onNameChange={(v) => patch({ name: v })}
        onRoleChange={(v) => patch(v === "ceo" ? { role: v, reportsTo: "" } : { role: v })}
        onReportsToChange={(v) => patch({ reportsTo: v })}
      />
      <CapabilityPreviewCard agent={agent} role={form.role} />
      <OrchestrationCard
        budgetCents={form.budgetMonthlyCents}
        maxConcurrent={form.maxConcurrentSessions}
        executorType={form.executorType}
        executorTypes={executorTypes}
        onBudgetChange={(v) => patch({ budgetMonthlyCents: v })}
        onMaxConcurrentChange={(v) => patch({ maxConcurrentSessions: v })}
        onExecutorChange={(v) => patch({ executorType: v })}
      />
      <AgentRoutingCard agentId={agent.id} />
      {dirty && (
        <div className="flex justify-end">
          <Button onClick={handleSave} disabled={saving} className="cursor-pointer">
            {saving ? t("office:saving") : t("office:saveConfiguration")}
          </Button>
        </div>
      )}
    </div>
  );
}

function reconcileForm(
  previousForm: FormState,
  nextForm: FormState,
  dirtyFields: Set<FormField>,
): FormState {
  const reconciled = { ...previousForm };
  for (const field of FORM_FIELDS) {
    if (!dirtyFields.has(field)) {
      Object.assign(reconciled, { [field]: nextForm[field] });
    }
  }
  return reconciled;
}

function buildAgentUpdate(
  form: FormState,
  canonical: FormState,
  dirtyFields: Set<FormField>,
): Partial<AgentProfile> {
  function valueFor<T extends FormField>(field: T): FormState[T] {
    return dirtyFields.has(field) ? form[field] : canonical[field];
  }

  const executorType = valueFor("executorType");
  return {
    name: valueFor("name"),
    role: valueFor("role"),
    reportsTo: valueFor("reportsTo"),
    budgetMonthlyCents: valueFor("budgetMonthlyCents"),
    maxConcurrentSessions: valueFor("maxConcurrentSessions"),
    executorPreference: executorType ? { type: executorType } : undefined,
  };
}

function useAgentConfigurationForm(agent: AgentProfile) {
  const { t } = useTranslation();
  const updateStore = useAppStore((s) => s.updateOfficeAgentProfile);
  const [form, setForm] = useState<FormState>(() => initialForm(agent));
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);
  const dirtyFieldsRef = useRef<Set<FormField>>(new Set());
  const previousAgentRef = useRef(agent);
  const activeAgentIdRef = useRef(agent.id);
  const saveRequestRef = useRef(0);
  const editGenerationRef = useRef(0);

  activeAgentIdRef.current = agent.id;

  useEffect(() => {
    const previousAgent = previousAgentRef.current;
    previousAgentRef.current = agent;
    const nextForm = initialForm(agent);

    if (previousAgent.id !== agent.id) {
      dirtyFieldsRef.current.clear();
      setForm(nextForm);
      setDirty(false);
      setSaving(false);
      return;
    }

    if (dirtyFieldsRef.current.size === 0) {
      setForm(nextForm);
      return;
    }

    setForm((previousForm) => reconcileForm(previousForm, nextForm, dirtyFieldsRef.current));
  }, [agent]);

  const patch = useCallback((p: Partial<FormState>) => {
    editGenerationRef.current += 1;
    for (const field of Object.keys(p) as FormField[]) {
      dirtyFieldsRef.current.add(field);
    }
    setForm((prev) => ({ ...prev, ...p }));
    setDirty(true);
  }, []);

  const handleSave = useCallback(async () => {
    const targetAgentId = agent.id;
    const requestId = saveRequestRef.current + 1;
    saveRequestRef.current = requestId;
    const canonical = initialForm(agent);
    const dirtyFields = dirtyFieldsRef.current;
    const editGeneration = editGenerationRef.current;

    setSaving(true);
    try {
      const update = buildAgentUpdate(form, canonical, dirtyFields);
      const saved = await updateAgentProfile(targetAgentId, update);
      updateStore(agent.workspaceId, targetAgentId, saved);
      if (
        activeAgentIdRef.current !== targetAgentId ||
        saveRequestRef.current !== requestId ||
        editGenerationRef.current !== editGeneration
      ) {
        return;
      }
      dirtyFieldsRef.current.clear();
      setForm(initialForm(saved));
      setDirty(false);
      toast.success(t("office:agentConfigurationUpdated"));
    } catch (err) {
      toast.error(
        agentValidationMessage(err, t) ??
          (err instanceof Error ? err.message : t("office:failedToUpdateAgent")),
      );
    } finally {
      if (activeAgentIdRef.current === targetAgentId && saveRequestRef.current === requestId) {
        setSaving(false);
      }
    }
  }, [agent, form, t, updateStore]);

  return { form, saving, dirty, patch, handleSave };
}

function CapabilityPreviewCard({ agent, role }: { agent: AgentProfile; role: AgentRole }) {
  const { t } = useTranslation();
  const capabilities = effectiveCapabilities(agent, role);
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">{t("office:runtimeCapabilities")}</CardTitle>
        <p className="text-xs text-muted-foreground">
          {t("office:effectiveActionsThisAgentCanRequest")}
        </p>
      </CardHeader>
      <CardContent>
        <div className="flex flex-wrap gap-2" data-testid="agent-capability-preview">
          {capabilities.map((key) => (
            <Badge key={key} variant="secondary">
              {/* `?? key` keeps an unmapped wire capability visible. */}
              {CAPABILITY_LABEL_KEYS[key] ? t(CAPABILITY_LABEL_KEYS[key]) : key}
            </Badge>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

function effectiveCapabilities(agent: AgentProfile, role: AgentRole): string[] {
  const permissions = agent.permissions ?? {};
  const allowed = new Set<string>([
    "post_comment",
    "update_task_status",
    "create_subtask",
    "read_memory",
    "write_memory",
    "list_skills",
  ]);
  if (role === "ceo" || permissions.can_create_agents === true) allowed.add("create_agent");
  if (permissions.can_approve === true) allowed.add("request_approval");
  if (permissions.can_spawn_agent_run === true) allowed.add("spawn_agent_run");
  if (permissions.can_modify_agents === true) allowed.add("modify_agents");
  if (permissions.can_delete_skills === true) allowed.add("delete_skills");
  return Object.keys(CAPABILITY_LABEL_KEYS).filter((key) => allowed.has(key));
}

function ReportsToField({
  reportsTo,
  choices,
  disabled,
  onChange,
}: {
  reportsTo: string;
  choices: Array<{ id: string; name: string }>;
  disabled: boolean;
  onChange: (v: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex-1">
      <Label htmlFor="cfg-reports-to">{t("office:reportsTo")}</Label>
      <Select
        value={reportsTo || "__none__"}
        disabled={disabled}
        onValueChange={(v) => onChange(v === "__none__" ? "" : v)}
      >
        <SelectTrigger
          id="cfg-reports-to"
          className="mt-1 min-h-11 w-full cursor-pointer md:min-h-7"
        >
          <SelectValue placeholder={t("office:noneTopLevel")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="__none__" className="cursor-pointer">
            {t("office:none")}
          </SelectItem>
          {choices.map((c) => (
            <SelectItem key={c.id} value={c.id} className="cursor-pointer">
              {c.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground mt-1">{t("office:whichAgentManagesThisOne")}</p>
    </div>
  );
}

function IdentityCard({
  name,
  role,
  roles,
  reportsTo,
  reportsToChoices,
  onNameChange,
  onRoleChange,
  onReportsToChange,
}: {
  name: string;
  role: AgentRole;
  roles: Array<{ id: string; label: string }>;
  reportsTo: string;
  reportsToChoices: Array<{ id: string; name: string }>;
  onNameChange: (v: string) => void;
  onRoleChange: (v: AgentRole) => void;
  onReportsToChange: (v: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">{t("office:identity")}</CardTitle>
        <p className="text-xs text-muted-foreground">
          {t("office:nameRoleAndReportingStructureFor")}
        </p>
      </CardHeader>
      <CardContent className="space-y-3">
        <div>
          <Label htmlFor="cfg-name">{t("office:name")}</Label>
          <Input
            id="cfg-name"
            value={name}
            onChange={(e) => onNameChange(e.target.value)}
            className="mt-1"
          />
        </div>
        <div className="flex flex-col gap-4 md:flex-row">
          <div className="flex-1">
            <Label>{t("office:role")}</Label>
            <Select value={role} onValueChange={(v) => onRoleChange(v as AgentRole)}>
              <SelectTrigger className="mt-1 min-h-11 w-full cursor-pointer md:min-h-7">
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
          </div>
          <ReportsToField
            reportsTo={reportsTo}
            choices={reportsToChoices}
            disabled={role === "ceo"}
            onChange={onReportsToChange}
          />
        </div>
      </CardContent>
    </Card>
  );
}

function OrchestrationCard({
  budgetCents,
  maxConcurrent,
  executorType,
  executorTypes,
  onBudgetChange,
  onMaxConcurrentChange,
  onExecutorChange,
}: {
  budgetCents: number;
  maxConcurrent: number;
  executorType: string;
  executorTypes: Array<{ id: string; label: string }>;
  onBudgetChange: (v: number) => void;
  onMaxConcurrentChange: (v: number) => void;
  onExecutorChange: (v: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">{t("office:orchestration")}</CardTitle>
        <p className="text-xs text-muted-foreground">
          {t("office:budgetCapConcurrencyAndExecutionEnvironment")}
        </p>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex gap-4">
          <div className="flex-1">
            <Label>{t("office:monthlyBudget")}</Label>
            <Input
              type="number"
              min={0}
              value={budgetCents / 100}
              onChange={(e) => onBudgetChange(Math.round(Number(e.target.value) * 100))}
              className="mt-1"
            />
          </div>
          <div className="flex-1">
            <Label>{t("office:maxConcurrentSessions")}</Label>
            <Input
              type="number"
              min={1}
              max={10}
              value={maxConcurrent}
              onChange={(e) => onMaxConcurrentChange(Number(e.target.value))}
              className="mt-1"
            />
          </div>
        </div>
        <div>
          <Label>{t("office:executorPreference")}</Label>
          <Select
            value={executorType || "__inherit__"}
            onValueChange={(v) => onExecutorChange(v === "__inherit__" ? "" : v)}
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
        </div>
      </CardContent>
    </Card>
  );
}

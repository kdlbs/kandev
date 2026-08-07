"use client";

import { useState, useCallback } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Button } from "@kandev/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { IconRefresh } from "@tabler/icons-react";
import { toast } from "@/lib/toast/sonner";
import { useAppStore } from "@/components/state-provider";
import { updateAgentProfile, getAgentUtilization } from "@/lib/api/domains/office-api";
import type { AgentProfile, AgentRole, ProviderUsage } from "@/lib/state/slices/office/types";
import { UtilizationBars } from "@/app/office/components/utilization-bars";
import { useTranslation } from "react-i18next";

type AgentOverviewTabProps = {
  agent: AgentProfile;
};

function IdentityCard({
  name,
  role,
  reportsToName,
  roles,
  onNameChange,
  onRoleChange,
}: {
  name: string;
  role: AgentRole;
  reportsToName: string;
  roles: Array<{ id: string; label: string }>;
  onNameChange: (v: string) => void;
  onRoleChange: (v: AgentRole) => void;
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
          <Label>{t("office:name")}</Label>
          <Input value={name} onChange={(e) => onNameChange(e.target.value)} className="mt-1" />
        </div>
        <div className="flex gap-4">
          <div className="flex-1">
            <Label>{t("office:role")}</Label>
            <Select value={role} onValueChange={(v) => onRoleChange(v as AgentRole)}>
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
          </div>
          <div className="flex-1">
            <Label>{t("office:reportsTo")}</Label>
            <Input value={reportsToName} disabled className="mt-1" />
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function ConfigurationCard({
  budget,
  maxConcurrent,
  executorType,
  executorTypes,
  onBudgetChange,
  onMaxConcurrentChange,
  onExecutorTypeChange,
}: {
  budget: number;
  maxConcurrent: number;
  executorType: string;
  executorTypes: Array<{ id: string; label: string }>;
  onBudgetChange: (v: number) => void;
  onMaxConcurrentChange: (v: number) => void;
  onExecutorTypeChange: (v: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">{t("office:configuration")}</CardTitle>
        <p className="text-xs text-muted-foreground">
          {t("office:budgetLimitsConcurrencyAndExecutionEnvironment")}
        </p>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex gap-4">
          <div className="flex-1">
            <Label>{t("office:monthlyBudget")}</Label>
            <Input
              type="number"
              min={0}
              value={budget}
              onChange={(e) => onBudgetChange(Number(e.target.value))}
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
            onValueChange={(v) => onExecutorTypeChange(v === "__inherit__" ? "" : v)}
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

function QuotaCard({
  agentId,
  initialUsage,
}: {
  agentId: string;
  initialUsage: ProviderUsage | null | undefined;
}) {
  const { t } = useTranslation();
  const [usage, setUsage] = useState<ProviderUsage | null>(initialUsage ?? null);
  const [refreshing, setRefreshing] = useState(false);

  const handleRefresh = useCallback(async () => {
    setRefreshing(true);
    try {
      const result = await getAgentUtilization(agentId);
      setUsage(result.utilization);
    } catch {
      toast.error(t("office:failedToRefreshUtilization"));
    } finally {
      setRefreshing(false);
    }
  }, [agentId]);

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="text-sm">{t("office:subscriptionQuota")}</CardTitle>
            <p className="text-xs text-muted-foreground mt-1">
              {t("office:currentUtilizationOfSubscriptionRateLimit")}
            </p>
          </div>
          <Button
            variant="ghost"
            size="icon"
            onClick={handleRefresh}
            disabled={refreshing}
            className="cursor-pointer h-7 w-7"
            title={t("office:refreshUtilization")}
          >
            <IconRefresh className={`h-4 w-4 ${refreshing ? "animate-spin" : ""}`} />
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {usage ? (
          <UtilizationBars usage={usage} />
        ) : (
          <p className="text-xs text-muted-foreground">
            {t("office:noUtilizationDataClickRefreshTo")}
          </p>
        )}
      </CardContent>
    </Card>
  );
}

// Catalog keys, not copy — module scope freezes a `t()` at the boot locale.
// The `id`s are the wire role / executor-type values.
const FALLBACK_ROLES = [
  { id: "ceo", labelKey: "office:roleCeo" },
  { id: "worker", labelKey: "office:roleWorker" },
  { id: "specialist", labelKey: "office:roleSpecialist" },
  { id: "assistant", labelKey: "office:roleAssistant" },
];

const FALLBACK_EXECUTOR_TYPES = [
  { id: "local_pc", labelKey: "office:localStandalone" },
  { id: "local_docker", labelKey: "office:localDocker" },
  { id: "sprites", labelKey: "office:spritesRemoteSandbox" },
];

export function AgentOverviewTab({ agent }: AgentOverviewTabProps) {
  const { t } = useTranslation();
  const agents = useAppStore((s) => s.office.agentProfiles);
  const meta = useAppStore((s) => s.office.meta);
  const updateStore = useAppStore((s) => s.updateOfficeAgentProfile);

  const roles =
    meta?.roles.map((r) => ({ id: r.id, label: r.label })) ??
    FALLBACK_ROLES.map((r) => ({ id: r.id, label: t(r.labelKey) }));
  const executorTypes =
    meta?.executorTypes.map((e) => ({ id: e.id, label: e.label })) ??
    FALLBACK_EXECUTOR_TYPES.map((e) => ({ id: e.id, label: t(e.labelKey) }));

  const [name, setName] = useState(agent.name);
  const [role, setRole] = useState<AgentRole>(agent.role);
  const [budget, setBudget] = useState(agent.budgetMonthlyCents / 100);
  const [maxConcurrent, setMaxConcurrent] = useState(agent.maxConcurrentSessions);
  const [executorType, setExecutorType] = useState(agent.executorPreference?.type ?? "");
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);

  const markDirty = useCallback(() => setDirty(true), []);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      await updateAgentProfile(agent.id, {
        name,
        role,
        budgetMonthlyCents: Math.round(budget * 100),
        maxConcurrentSessions: maxConcurrent,
        executorPreference: executorType ? { type: executorType } : undefined,
      } as Partial<AgentProfile>);
      updateStore(agent.id, {
        name,
        role,
        budgetMonthlyCents: Math.round(budget * 100),
        maxConcurrentSessions: maxConcurrent,
        executorPreference: executorType ? { type: executorType } : undefined,
      });
      setDirty(false);
      toast.success(t("office:agentUpdated"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("office:failedToUpdateAgent"));
    } finally {
      setSaving(false);
    }
  }, [agent.id, name, role, budget, maxConcurrent, executorType, updateStore]);

  const reportsToAgent = agents.find((a) => a.id === agent.reportsTo);

  const isSubscription = agent.billingType === "subscription";

  return (
    <div className="space-y-4 mt-4">
      <IdentityCard
        name={name}
        role={role}
        reportsToName={reportsToAgent?.name ?? t("office:none")}
        roles={roles}
        onNameChange={(v) => {
          setName(v);
          markDirty();
        }}
        onRoleChange={(v) => {
          setRole(v);
          markDirty();
        }}
      />
      <ConfigurationCard
        budget={budget}
        maxConcurrent={maxConcurrent}
        executorType={executorType}
        executorTypes={executorTypes}
        onBudgetChange={(v) => {
          setBudget(v);
          markDirty();
        }}
        onMaxConcurrentChange={(v) => {
          setMaxConcurrent(v);
          markDirty();
        }}
        onExecutorTypeChange={(v) => {
          setExecutorType(v);
          markDirty();
        }}
      />
      {isSubscription && <QuotaCard agentId={agent.id} initialUsage={agent.utilization} />}
      {dirty && (
        <div className="flex justify-end">
          <Button onClick={handleSave} disabled={saving} className="cursor-pointer">
            {saving ? t("office:saving") : t("office:saveChanges")}
          </Button>
        </div>
      )}
    </div>
  );
}

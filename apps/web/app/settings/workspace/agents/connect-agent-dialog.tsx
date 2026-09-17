"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@kandev/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useWorkspaceRouting } from "@/hooks/domains/office/use-workspace-routing";
import { notifyWorkspaceAgentsChanged } from "@/hooks/domains/office/use-workspace-agents";
import { connectWorkspaceAgent } from "@/lib/api/domains/workspace-agents-api";
import { PersonaExecutorField } from "@/app/office/agents/[id]/components/persona-executor-field";
import type { AgentRole } from "@/lib/types/agent-profile";
import { toast } from "@/lib/toast/sonner";

export function ConnectAgentDialog({
  workspaceId,
  open,
  onClose,
}: {
  workspaceId: string;
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const routing = useWorkspaceRouting(workspaceId);
  const [role, setRole] = useState<AgentRole>("assistant");
  const [name, setName] = useState("");
  const [delegationContext, setDelegationContext] = useState("");
  const [profileId, setProfileId] = useState("");
  const [executorId, setExecutorId] = useState("");
  const [executorType, setExecutorType] = useState("");
  const [saving, setSaving] = useState(false);
  const connect = async () => {
    setSaving(true);
    try {
      await connectWorkspaceAgent(workspaceId, {
        name: name.trim(),
        delegationContext,
        role,
        profileId,
        executorId,
        executorType,
      });
      notifyWorkspaceAgentsChanged();
      onClose();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };
  return (
    <Dialog
      open={open}
      onOpenChange={(value) => {
        if (!value && !saving) onClose();
      }}
    >
      <DialogContent className="max-w-lg max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{t("office:connectWorkspaceAgent")}</DialogTitle>
        </DialogHeader>
        <p className="text-sm text-muted-foreground">{t("office:connectAgentHint")}</p>
        <Label>{t("office:role")}</Label>
        <RoleChoice value={role} onChange={setRole} />
        <Label htmlFor="connection-name">{t("office:name")}</Label>
        <Input id="connection-name" value={name} onChange={(e) => setName(e.target.value)} />
        <DelegationChoice value={delegationContext} onChange={setDelegationContext} />
        <Label>{t("office:executionProfileChoice")}</Label>
        <Select value={profileId} onValueChange={setProfileId}>
          <SelectTrigger className="cursor-pointer" data-testid="connection-profile">
            <SelectValue placeholder={t("office:chooseExecutionProfile")} />
          </SelectTrigger>
          <SelectContent>
            {routing.executionProfiles.map((p) => (
              <SelectItem key={p.id} value={p.id}>
                {p.name} ({p.provider_id}, {p.model})
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">{t("office:executionProfileChoiceHint")}</p>
        {routing.error && <p role="alert">{routing.error}</p>}
        <PersonaExecutorField
          value={executorId}
          onChange={(id, type) => {
            setExecutorId(id);
            setExecutorType(type);
          }}
        />
        <p className="text-sm text-muted-foreground">{t("office:manualAgentSetupHint")}</p>
        <DialogFooter>
          <Button
            className="cursor-pointer"
            onClick={connect}
            disabled={
              saving ||
              !name.trim() ||
              !routing.executionProfiles.some((p) => p.id === profileId) ||
              !executorId
            }
          >
            {t(saving ? "office:creating" : "office:connectWorkspaceAgent")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function RoleChoice({
  value,
  onChange,
}: {
  value: AgentRole;
  onChange: (role: AgentRole) => void;
}) {
  const { t } = useTranslation();
  return (
    <Select value={value} onValueChange={(v) => onChange(v as AgentRole)}>
      <SelectTrigger className="cursor-pointer">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {(["assistant", "worker", "specialist", "qa", "security", "devops"] as const).map((r) => (
          <SelectItem key={r} value={r}>
            {t(`office:setupRole_${r}`)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function DelegationChoice({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <Label htmlFor="delegation-context">{t("office:delegationContext")}</Label>
      <Textarea
        id="delegation-context"
        value={value}
        maxLength={2000}
        onChange={(e) => onChange(e.target.value)}
        placeholder={t("office:delegationContextExample")}
      />
    </div>
  );
}

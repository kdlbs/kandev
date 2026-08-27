"use client";

import { useState, useCallback } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Switch } from "@kandev/ui/switch";
import { Label } from "@kandev/ui/label";
import { Input } from "@kandev/ui/input";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { toast } from "@/lib/toast/sonner";
import { useAppStore } from "@/components/state-provider";
import { updateAgentProfile } from "@/lib/api/domains/office-api";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { useTranslation } from "react-i18next";

type AgentPermissionsTabProps = {
  agent: AgentProfile;
};

export function AgentPermissionsTab({ agent }: AgentPermissionsTabProps) {
  const { t } = useTranslation();
  const meta = useAppStore((s) => s.office.meta);
  const updateStore = useAppStore((s) => s.updateOfficeAgentProfile);

  const permDefs = meta?.permissions ?? [];
  const roleDefaults = meta?.permissionDefaults?.[agent.role] ?? {};

  const [perms, setPerms] = useState<Record<string, unknown>>(
    () => (agent.permissions as Record<string, unknown>) ?? {},
  );
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);

  const updatePerm = useCallback((key: string, value: unknown) => {
    setPerms((prev) => ({ ...prev, [key]: value }));
    setDirty(true);
  }, []);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      await updateAgentProfile(agent.id, {
        permissions: perms,
      } as Partial<AgentProfile>);
      updateStore(agent.workspaceId, agent.id, { permissions: perms });
      setDirty(false);
      toast.success(t("office:permissionsUpdated"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("office:failedToSave"));
    } finally {
      setSaving(false);
    }
  }, [agent.id, agent.workspaceId, perms, updateStore]);

  const isDefault = (key: string) => {
    const current = perms[key];
    const def = roleDefaults[key];
    if (current === undefined) return true;
    return current === def;
  };

  return (
    <div className="space-y-4 mt-4">
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">{t("office:permissions")}</CardTitle>
          <p className="text-xs text-muted-foreground">
            {t("office:controlWhatThisAgentIsAllowed")}
          </p>
        </CardHeader>
        <CardContent className="space-y-4">
          {permDefs.map((def) => (
            <PermissionRow
              key={def.key}
              permKey={def.key}
              label={def.label}
              description={def.description}
              type={def.type}
              value={perms[def.key] ?? roleDefaults[def.key]}
              isDefault={isDefault(def.key)}
              onChange={(v) => updatePerm(def.key, v)}
            />
          ))}
        </CardContent>
      </Card>
      {dirty && (
        <div className="flex justify-end">
          <Button onClick={handleSave} disabled={saving} className="cursor-pointer">
            {saving ? t("office:saving") : t("office:savePermissions")}
          </Button>
        </div>
      )}
    </div>
  );
}

function PermissionRow({
  permKey,
  label,
  description,
  type,
  value,
  isDefault,
  onChange,
}: {
  permKey: string;
  label: string;
  description: string;
  type: string;
  value: unknown;
  isDefault: boolean;
  onChange: (v: unknown) => void;
}) {
  const { t } = useTranslation();
  if (type === "int") {
    return (
      <div className="flex items-center justify-between gap-4">
        <div className="flex-1">
          <div className="flex items-center gap-2">
            <Label htmlFor={permKey}>{label}</Label>
            {isDefault && <Badge variant="outline">{t("office:roleDefault")}</Badge>}
          </div>
          <p className="text-xs text-muted-foreground mt-0.5">{description}</p>
        </div>
        <Input
          id={permKey}
          type="number"
          min={0}
          max={10}
          className="w-20"
          value={typeof value === "number" ? value : 1}
          onChange={(e) => onChange(Number(e.target.value))}
        />
      </div>
    );
  }
  return (
    <div className="flex items-center justify-between gap-4">
      <div className="flex-1">
        <div className="flex items-center gap-2">
          <Label htmlFor={permKey}>{label}</Label>
          {isDefault && <Badge variant="outline">{t("office:roleDefault")}</Badge>}
        </div>
        <p className="text-xs text-muted-foreground mt-0.5">{description}</p>
      </div>
      <Switch
        id={permKey}
        checked={Boolean(value)}
        onCheckedChange={(checked) => onChange(checked)}
        className="cursor-pointer"
      />
    </div>
  );
}

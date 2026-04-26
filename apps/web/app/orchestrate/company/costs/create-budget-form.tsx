"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Card, CardContent } from "@kandev/ui/card";
import { createBudget } from "@/lib/api/domains/orchestrate-api";

type Props = {
  workspaceId: string;
  onCreated: () => void;
  onCancel: () => void;
};

function FormField({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="text-xs text-muted-foreground">{label}</label>
      {children}
    </div>
  );
}

export function CreateBudgetForm({ workspaceId, onCreated, onCancel }: Props) {
  const [scopeType, setScopeType] = useState("workspace");
  const [scopeId, setScopeId] = useState(workspaceId);
  const [limitDollars, setLimitDollars] = useState("");
  const [period, setPeriod] = useState("monthly");
  const [alertPct, setAlertPct] = useState("80");
  const [action, setAction] = useState("notify_only");
  const [saving, setSaving] = useState(false);

  const handleSubmit = async () => {
    setSaving(true);
    try {
      await createBudget(workspaceId, {
        scopeType: scopeType as "agent" | "project" | "workspace",
        scopeId,
        limitCents: Math.round(parseFloat(limitDollars || "0") * 100),
        period: period as "monthly" | "total",
        alertThresholdPct: parseInt(alertPct, 10) || 80,
        actionOnExceed: action as "notify_only" | "pause_agent" | "block_new_tasks",
      });
      onCreated();
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card>
      <CardContent className="pt-4 space-y-3">
        <div className="grid grid-cols-2 gap-3">
          <FormField label="Scope Type">
            <Select value={scopeType} onValueChange={setScopeType}>
              <SelectTrigger className="h-8 text-sm"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="workspace">Workspace</SelectItem>
                <SelectItem value="agent">Agent</SelectItem>
                <SelectItem value="project">Project</SelectItem>
              </SelectContent>
            </Select>
          </FormField>
          <FormField label="Scope ID">
            <Input className="h-8 text-sm" value={scopeId} onChange={(e) => setScopeId(e.target.value)} placeholder="Entity ID" />
          </FormField>
          <FormField label="Limit ($)">
            <Input className="h-8 text-sm" type="number" value={limitDollars} onChange={(e) => setLimitDollars(e.target.value)} placeholder="10.00" />
          </FormField>
          <FormField label="Period">
            <Select value={period} onValueChange={setPeriod}>
              <SelectTrigger className="h-8 text-sm"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="monthly">Monthly</SelectItem>
                <SelectItem value="total">Total</SelectItem>
              </SelectContent>
            </Select>
          </FormField>
          <FormField label="Alert Threshold (%)">
            <Input className="h-8 text-sm" type="number" value={alertPct} onChange={(e) => setAlertPct(e.target.value)} />
          </FormField>
          <FormField label="Action on Exceed">
            <Select value={action} onValueChange={setAction}>
              <SelectTrigger className="h-8 text-sm"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="notify_only">Notify Only</SelectItem>
                <SelectItem value="pause_agent">Pause Agent</SelectItem>
                <SelectItem value="block_new_tasks">Block New Tasks</SelectItem>
              </SelectContent>
            </Select>
          </FormField>
        </div>
        <div className="flex justify-end gap-2 pt-2">
          <Button variant="ghost" size="sm" className="cursor-pointer" onClick={onCancel}>Cancel</Button>
          <Button size="sm" className="cursor-pointer" disabled={saving} onClick={handleSubmit}>
            {saving ? "Creating..." : "Create Policy"}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

"use client";

import { useState } from "react";
import { Input } from "@kandev/ui/input";
import { Checkbox } from "@kandev/ui/checkbox";
import { Separator } from "@kandev/ui/separator";
import { ConfigSection } from "./config-section";

export function SettingsContent() {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [approvalNewAgents, setApprovalNewAgents] = useState(true);
  const [approvalTaskCompletion, setApprovalTaskCompletion] = useState(false);
  const [approvalSkillChanges, setApprovalSkillChanges] = useState(true);

  return (
    <div className="max-w-3xl mx-auto p-6 space-y-8">
      <h1 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
        Settings
      </h1>

      <section className="space-y-4">
        <h2 className="text-sm font-semibold">Workspace</h2>
        <div>
          <label className="text-sm font-medium">Name</label>
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Workspace name"
            className="mt-1"
          />
        </div>
        <div>
          <label className="text-sm font-medium">Description</label>
          <Input
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Optional description"
            className="mt-1"
          />
        </div>
      </section>

      <Separator />

      <section className="space-y-4">
        <h2 className="text-sm font-semibold">Approval Settings</h2>
        <label className="flex items-center gap-3 cursor-pointer">
          <Checkbox
            checked={approvalNewAgents}
            onCheckedChange={(v) => setApprovalNewAgents(!!v)}
          />
          <span className="text-sm">Require approval for new agents</span>
        </label>
        <label className="flex items-center gap-3 cursor-pointer">
          <Checkbox
            checked={approvalTaskCompletion}
            onCheckedChange={(v) => setApprovalTaskCompletion(!!v)}
          />
          <span className="text-sm">Require approval for task completion</span>
        </label>
        <label className="flex items-center gap-3 cursor-pointer">
          <Checkbox
            checked={approvalSkillChanges}
            onCheckedChange={(v) => setApprovalSkillChanges(!!v)}
          />
          <span className="text-sm">Require approval for skill changes</span>
        </label>
      </section>

      <Separator />

      <ConfigSection />
    </div>
  );
}

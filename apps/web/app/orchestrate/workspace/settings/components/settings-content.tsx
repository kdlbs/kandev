"use client";

import { useRef, useState } from "react";
import { IconUpload } from "@tabler/icons-react";
import { Input } from "@kandev/ui/input";
import { Switch } from "@kandev/ui/switch";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { ConfigSection } from "./config-section";

function SectionHeader({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-3 mb-3">
      <h2 className="text-[10px] font-medium uppercase tracking-widest font-mono text-muted-foreground/60 shrink-0">
        {children}
      </h2>
      <div className="h-px bg-border flex-1" />
    </div>
  );
}

function SettingCard({ children }: { children: React.ReactNode }) {
  return (
    <div className="rounded-lg border border-border p-4 space-y-4">
      {children}
    </div>
  );
}

function ToggleRow({ label, description, checked, onCheckedChange }: {
  label: string;
  description?: string;
  checked: boolean;
  onCheckedChange: (v: boolean) => void;
}) {
  return (
    <div className="flex items-center justify-between gap-4">
      <div>
        <p className="text-sm">{label}</p>
        {description && <p className="text-xs text-muted-foreground mt-0.5">{description}</p>}
      </div>
      <Switch checked={checked} onCheckedChange={onCheckedChange} className="cursor-pointer" />
    </div>
  );
}

export function SettingsContent() {
  const workspaces = useAppStore((s) => s.workspaces);
  const activeWorkspace = workspaces.items.find((w) => w.id === workspaces.activeId);

  const [name, setName] = useState(activeWorkspace?.name || "");
  const [description, setDescription] = useState(activeWorkspace?.description || "");
  const [logoPreview, setLogoPreview] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [approvalNewAgents, setApprovalNewAgents] = useState(true);
  const [approvalTaskCompletion, setApprovalTaskCompletion] = useState(false);
  const [approvalSkillChanges, setApprovalSkillChanges] = useState(true);

  const initial = (name || "W").charAt(0).toUpperCase();

  const handleLogoChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      const url = URL.createObjectURL(file);
      setLogoPreview(url);
    }
  };

  return (
    <div className="max-w-3xl mx-auto p-6 space-y-8">
      {/* Appearance */}
      <div>
        <SectionHeader>Appearance</SectionHeader>
        <SettingCard>
          <div className="flex items-center gap-4">
            <div className="h-14 w-14 rounded-xl bg-primary text-primary-foreground flex items-center justify-center text-lg font-semibold shrink-0 overflow-hidden">
              {logoPreview ? (
                <img src={logoPreview} alt="Logo" className="h-full w-full object-cover" />
              ) : (
                initial
              )}
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-sm text-muted-foreground mb-2">Logo</p>
              <Button
                variant="outline"
                size="sm"
                className="cursor-pointer"
                onClick={() => fileInputRef.current?.click()}
              >
                <IconUpload className="h-3.5 w-3.5 mr-1.5" />
                Upload logo
              </Button>
              <input
                ref={fileInputRef}
                type="file"
                accept="image/*"
                onChange={handleLogoChange}
                className="hidden"
              />
            </div>
          </div>
          <div>
            <label className="text-sm text-muted-foreground">Name</label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Workspace name"
              className="mt-1"
            />
          </div>
          <div>
            <label className="text-sm text-muted-foreground">Description</label>
            <Input
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Optional description"
              className="mt-1"
            />
          </div>
        </SettingCard>
      </div>

      {/* Permissions */}
      <div>
        <SectionHeader>Permissions</SectionHeader>
        <SettingCard>
          <ToggleRow
            label="Require approval for new agents"
            description="New agent hires must be approved before activation"
            checked={approvalNewAgents}
            onCheckedChange={setApprovalNewAgents}
          />
          <ToggleRow
            label="Require approval for task completion"
            description="Tasks must be reviewed before they can be marked as done"
            checked={approvalTaskCompletion}
            onCheckedChange={setApprovalTaskCompletion}
          />
          <ToggleRow
            label="Require approval for skill changes"
            description="Agent-created skills must be approved before activation"
            checked={approvalSkillChanges}
            onCheckedChange={setApprovalSkillChanges}
          />
        </SettingCard>
      </div>

      {/* Configuration */}
      <div>
        <SectionHeader>Configuration</SectionHeader>
        <SettingCard>
          <ConfigSection />
        </SettingCard>
      </div>
    </div>
  );
}

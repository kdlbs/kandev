"use client";

import { useCallback, useRef, useState } from "react";
import Image from "next/image";
import { IconUpload, IconDeviceFloppy } from "@tabler/icons-react";
import { toast } from "sonner";
import { Input } from "@kandev/ui/input";
import { Switch } from "@kandev/ui/switch";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { updateWorkspaceSettings } from "@/lib/api/domains/orchestrate-api";
import { ConfigSection } from "./config-section";
import { GitSection } from "./git-section";

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
  return <div className="rounded-lg border border-border p-4 space-y-4">{children}</div>;
}

function ToggleRow({
  label,
  description,
  checked,
  onCheckedChange,
}: {
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

function AppearanceSection({
  name,
  description,
  logoPreview,
  initial,
  fileInputRef,
  dirty,
  saving,
  onNameChange,
  onDescriptionChange,
  onLogoChange,
  onSave,
}: {
  name: string;
  description: string;
  logoPreview: string | null;
  initial: string;
  fileInputRef: React.RefObject<HTMLInputElement | null>;
  dirty: boolean;
  saving: boolean;
  onNameChange: (v: string) => void;
  onDescriptionChange: (v: string) => void;
  onLogoChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  onSave: () => void;
}) {
  return (
    <SettingCard>
      <div className="flex items-center gap-4">
        <div className="h-14 w-14 rounded-xl bg-primary text-primary-foreground flex items-center justify-center text-lg font-semibold shrink-0 overflow-hidden">
          {logoPreview ? (
            <Image
              src={logoPreview}
              alt="Logo"
              width={56}
              height={56}
              className="h-full w-full object-cover"
              unoptimized
            />
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
            onChange={onLogoChange}
            className="hidden"
          />
        </div>
      </div>
      <div>
        <label className="text-sm text-muted-foreground">Name</label>
        <Input
          value={name}
          onChange={(e) => onNameChange(e.target.value)}
          placeholder="Workspace name"
          className="mt-1"
        />
      </div>
      <div>
        <label className="text-sm text-muted-foreground">Description</label>
        <Input
          value={description}
          onChange={(e) => onDescriptionChange(e.target.value)}
          placeholder="Optional description"
          className="mt-1"
        />
      </div>
      {dirty && (
        <div className="flex justify-end pt-2">
          <Button size="sm" onClick={onSave} disabled={saving} className="cursor-pointer">
            <IconDeviceFloppy className="h-4 w-4 mr-1.5" />
            {saving ? "Saving..." : "Save"}
          </Button>
        </div>
      )}
    </SettingCard>
  );
}

function PermissionsSection({
  approvalNewAgents,
  approvalTaskCompletion,
  approvalSkillChanges,
  dirty,
  saving,
  onApprovalNewAgentsChange,
  onApprovalTaskCompletionChange,
  onApprovalSkillChangesChange,
  onSave,
}: {
  approvalNewAgents: boolean;
  approvalTaskCompletion: boolean;
  approvalSkillChanges: boolean;
  dirty: boolean;
  saving: boolean;
  onApprovalNewAgentsChange: (v: boolean) => void;
  onApprovalTaskCompletionChange: (v: boolean) => void;
  onApprovalSkillChangesChange: (v: boolean) => void;
  onSave: () => void;
}) {
  return (
    <SettingCard>
      <ToggleRow
        label="Require approval for new agents"
        description="New agent hires must be approved before activation"
        checked={approvalNewAgents}
        onCheckedChange={onApprovalNewAgentsChange}
      />
      <ToggleRow
        label="Require approval for task completion"
        description="Tasks must be reviewed before they can be marked as done"
        checked={approvalTaskCompletion}
        onCheckedChange={onApprovalTaskCompletionChange}
      />
      <ToggleRow
        label="Require approval for skill changes"
        description="Agent-created skills must be approved before activation"
        checked={approvalSkillChanges}
        onCheckedChange={onApprovalSkillChangesChange}
      />
      {dirty && (
        <div className="flex justify-end pt-2">
          <Button size="sm" onClick={onSave} disabled={saving} className="cursor-pointer">
            <IconDeviceFloppy className="h-4 w-4 mr-1.5" />
            {saving ? "Saving..." : "Save"}
          </Button>
        </div>
      )}
    </SettingCard>
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
  const [savingAppearance, setSavingAppearance] = useState(false);
  const [savingPermissions, setSavingPermissions] = useState(false);

  const initial = (name || "W").charAt(0).toUpperCase();
  const origName = activeWorkspace?.name || "";
  const origDescription = activeWorkspace?.description || "";

  const appearanceDirty = name !== origName || description !== origDescription;
  const permissionsDirty = true; // Toggles have no server-sourced initial values yet

  const handleLogoChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      const url = URL.createObjectURL(file);
      setLogoPreview(url);
    }
  };

  const handleSaveAppearance = useCallback(async () => {
    if (!activeWorkspace) return;
    setSavingAppearance(true);
    try {
      await updateWorkspaceSettings(activeWorkspace.id, { name, description });
      toast.success("Appearance settings saved");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to save settings");
    } finally {
      setSavingAppearance(false);
    }
  }, [activeWorkspace, name, description]);

  const handleSavePermissions = useCallback(async () => {
    if (!activeWorkspace) return;
    setSavingPermissions(true);
    try {
      await updateWorkspaceSettings(activeWorkspace.id, {
        require_approval_for_new_agents: approvalNewAgents,
        require_approval_for_task_completion: approvalTaskCompletion,
        require_approval_for_skill_changes: approvalSkillChanges,
      });
      toast.success("Permission settings saved");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to save settings");
    } finally {
      setSavingPermissions(false);
    }
  }, [activeWorkspace, approvalNewAgents, approvalTaskCompletion, approvalSkillChanges]);

  return (
    <div className="max-w-3xl mx-auto p-6 space-y-8">
      <div>
        <SectionHeader>Appearance</SectionHeader>
        <AppearanceSection
          name={name}
          description={description}
          logoPreview={logoPreview}
          initial={initial}
          fileInputRef={fileInputRef}
          dirty={appearanceDirty}
          saving={savingAppearance}
          onNameChange={setName}
          onDescriptionChange={setDescription}
          onLogoChange={handleLogoChange}
          onSave={handleSaveAppearance}
        />
      </div>

      <div>
        <SectionHeader>Repository</SectionHeader>
        <SettingCard>
          <GitSection />
        </SettingCard>
      </div>

      <div>
        <SectionHeader>Permissions</SectionHeader>
        <PermissionsSection
          approvalNewAgents={approvalNewAgents}
          approvalTaskCompletion={approvalTaskCompletion}
          approvalSkillChanges={approvalSkillChanges}
          dirty={permissionsDirty}
          saving={savingPermissions}
          onApprovalNewAgentsChange={setApprovalNewAgents}
          onApprovalTaskCompletionChange={setApprovalTaskCompletion}
          onApprovalSkillChangesChange={setApprovalSkillChanges}
          onSave={handleSavePermissions}
        />
      </div>

      <div>
        <SectionHeader>Configuration</SectionHeader>
        <SettingCard>
          <ConfigSection />
        </SettingCard>
      </div>
    </div>
  );
}

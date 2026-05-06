"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Image from "next/image";
import { IconUpload, IconDeviceFloppy } from "@tabler/icons-react";
import { toast } from "sonner";
import { Input } from "@kandev/ui/input";
import { Switch } from "@kandev/ui/switch";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { updateWorkspaceSettings, getWorkspaceSettings } from "@/lib/api/domains/office-api";
import type { WorkspaceState } from "@/lib/state/slices/workspace/types";
import { ConfigSection } from "./config-section";
import { DangerZoneSection } from "./danger-zone-section";
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

function RecoverySection({
  lookbackHours,
  dirty,
  saving,
  onLookbackChange,
  onSave,
}: {
  lookbackHours: number;
  dirty: boolean;
  saving: boolean;
  onLookbackChange: (v: number) => void;
  onSave: () => void;
}) {
  return (
    <SettingCard>
      <div className="flex items-center justify-between gap-4">
        <div>
          <p className="text-sm">Recovery lookback window</p>
          <p className="text-xs text-muted-foreground mt-0.5">
            How far back to look for unstarted tasks during recovery sweeps
          </p>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <Input
            type="number"
            min={1}
            max={720}
            value={lookbackHours}
            onChange={(e) => {
              const v = parseInt(e.target.value, 10);
              if (!isNaN(v)) onLookbackChange(v);
            }}
            className="w-20 text-right"
          />
          <span className="text-sm text-muted-foreground">hours</span>
        </div>
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

type Workspace = WorkspaceState["items"][number];

function useRecoveryState(activeWorkspace: Workspace | undefined) {
  const [lookbackHours, setLookbackHours] = useState(24);
  const [origLookbackHours, setOrigLookbackHours] = useState(24);
  const [savingRecovery, setSavingRecovery] = useState(false);
  const activeWorkspaceId = activeWorkspace?.id;

  useEffect(() => {
    if (!activeWorkspaceId) return;
    void getWorkspaceSettings(activeWorkspaceId)
      .then((res) => {
        const hours = res.settings?.recovery_lookback_hours;
        if (hours && hours > 0) {
          setLookbackHours(hours);
          setOrigLookbackHours(hours);
        }
      })
      .catch(() => {});
  }, [activeWorkspaceId]);

  const handleSaveRecovery = useCallback(async () => {
    if (!activeWorkspace) return;
    const clamped = Math.max(1, Math.min(720, lookbackHours));
    setSavingRecovery(true);
    try {
      await updateWorkspaceSettings(activeWorkspace.id, { recovery_lookback_hours: clamped });
      setLookbackHours(clamped);
      setOrigLookbackHours(clamped);
      toast.success("Recovery settings saved");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to save settings");
    } finally {
      setSavingRecovery(false);
    }
  }, [activeWorkspace, lookbackHours]);

  return {
    lookbackHours,
    setLookbackHours,
    savingRecovery,
    recoveryDirty: lookbackHours !== origLookbackHours,
    handleSaveRecovery,
  };
}

function useSettingsState(activeWorkspace: Workspace | undefined) {
  const [name, setName] = useState(activeWorkspace?.name ?? "");
  const [description, setDescription] = useState(activeWorkspace?.description ?? "");
  const [logoPreview, setLogoPreview] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [approvalNewAgents, setApprovalNewAgents] = useState(true);
  const [approvalTaskCompletion, setApprovalTaskCompletion] = useState(false);
  const [approvalSkillChanges, setApprovalSkillChanges] = useState(true);
  const [origApprovalNewAgents, setOrigApprovalNewAgents] = useState(true);
  const [origApprovalTaskCompletion, setOrigApprovalTaskCompletion] = useState(false);
  const [origApprovalSkillChanges, setOrigApprovalSkillChanges] = useState(true);
  const [savingAppearance, setSavingAppearance] = useState(false);
  const [savingPermissions, setSavingPermissions] = useState(false);
  const recovery = useRecoveryState(activeWorkspace);

  const activeWorkspaceId = activeWorkspace?.id;

  useEffect(() => {
    if (!activeWorkspaceId) return;
    void getWorkspaceSettings(activeWorkspaceId)
      .then((res) => {
        const s = res.settings;
        if (s.require_approval_for_new_agents !== undefined) {
          setApprovalNewAgents(s.require_approval_for_new_agents);
          setOrigApprovalNewAgents(s.require_approval_for_new_agents);
        }
        if (s.require_approval_for_task_completion !== undefined) {
          setApprovalTaskCompletion(s.require_approval_for_task_completion);
          setOrigApprovalTaskCompletion(s.require_approval_for_task_completion);
        }
        if (s.require_approval_for_skill_changes !== undefined) {
          setApprovalSkillChanges(s.require_approval_for_skill_changes);
          setOrigApprovalSkillChanges(s.require_approval_for_skill_changes);
        }
      })
      .catch(() => {});
  }, [activeWorkspaceId]);

  const handleLogoChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) setLogoPreview(URL.createObjectURL(file));
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
      setOrigApprovalNewAgents(approvalNewAgents);
      setOrigApprovalTaskCompletion(approvalTaskCompletion);
      setOrigApprovalSkillChanges(approvalSkillChanges);
      toast.success("Permission settings saved");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to save settings");
    } finally {
      setSavingPermissions(false);
    }
  }, [activeWorkspace, approvalNewAgents, approvalTaskCompletion, approvalSkillChanges]);

  const origName = activeWorkspace?.name ?? "";
  const origDescription = activeWorkspace?.description ?? "";

  return {
    name,
    setName,
    description,
    setDescription,
    logoPreview,
    fileInputRef,
    approvalNewAgents,
    setApprovalNewAgents,
    approvalTaskCompletion,
    setApprovalTaskCompletion,
    approvalSkillChanges,
    setApprovalSkillChanges,
    ...recovery,
    appearanceDirty: name !== origName || description !== origDescription,
    permissionsDirty:
      approvalNewAgents !== origApprovalNewAgents ||
      approvalTaskCompletion !== origApprovalTaskCompletion ||
      approvalSkillChanges !== origApprovalSkillChanges,
    savingAppearance,
    savingPermissions,
    handleLogoChange,
    handleSaveAppearance,
    handleSavePermissions,
  };
}

export function SettingsContent() {
  const workspaces = useAppStore((s) => s.workspaces);
  const setWorkspaces = useAppStore((s) => s.setWorkspaces);
  const setActiveWorkspace = useAppStore((s) => s.setActiveWorkspace);
  const activeWorkspace = workspaces.items.find((w) => w.id === workspaces.activeId);
  const s = useSettingsState(activeWorkspace);
  const initial = (s.name || "W").charAt(0).toUpperCase();

  return (
    <div className="max-w-3xl mx-auto p-6 space-y-8">
      <div>
        <SectionHeader>Appearance</SectionHeader>
        <AppearanceSection
          name={s.name}
          description={s.description}
          logoPreview={s.logoPreview}
          initial={initial}
          fileInputRef={s.fileInputRef}
          dirty={s.appearanceDirty}
          saving={s.savingAppearance}
          onNameChange={s.setName}
          onDescriptionChange={s.setDescription}
          onLogoChange={s.handleLogoChange}
          onSave={s.handleSaveAppearance}
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
          approvalNewAgents={s.approvalNewAgents}
          approvalTaskCompletion={s.approvalTaskCompletion}
          approvalSkillChanges={s.approvalSkillChanges}
          dirty={s.permissionsDirty}
          saving={s.savingPermissions}
          onApprovalNewAgentsChange={s.setApprovalNewAgents}
          onApprovalTaskCompletionChange={s.setApprovalTaskCompletion}
          onApprovalSkillChangesChange={s.setApprovalSkillChanges}
          onSave={s.handleSavePermissions}
        />
      </div>

      <div>
        <SectionHeader>Recovery</SectionHeader>
        <RecoverySection
          lookbackHours={s.lookbackHours}
          dirty={s.recoveryDirty}
          saving={s.savingRecovery}
          onLookbackChange={s.setLookbackHours}
          onSave={s.handleSaveRecovery}
        />
      </div>

      <div>
        <SectionHeader>Configuration</SectionHeader>
        <SettingCard>
          <ConfigSection />
        </SettingCard>
      </div>

      {activeWorkspace && (
        <div>
          <SectionHeader>Danger Zone</SectionHeader>
          <DangerZoneSection
            workspace={activeWorkspace}
            workspaces={workspaces.items}
            setWorkspaces={setWorkspaces}
            setActiveWorkspace={setActiveWorkspace}
          />
        </div>
      )}
    </div>
  );
}

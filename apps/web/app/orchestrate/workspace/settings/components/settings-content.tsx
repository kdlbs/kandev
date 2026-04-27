"use client";

import { useRef, useState } from "react";
import Image from "next/image";
import { IconUpload } from "@tabler/icons-react";
import { Input } from "@kandev/ui/input";
import { Switch } from "@kandev/ui/switch";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
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
  onNameChange,
  onDescriptionChange,
  onLogoChange,
}: {
  name: string;
  description: string;
  logoPreview: string | null;
  initial: string;
  fileInputRef: React.RefObject<HTMLInputElement | null>;
  onNameChange: (v: string) => void;
  onDescriptionChange: (v: string) => void;
  onLogoChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
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
    </SettingCard>
  );
}

function PermissionsSection({
  approvalNewAgents,
  approvalTaskCompletion,
  approvalSkillChanges,
  onApprovalNewAgentsChange,
  onApprovalTaskCompletionChange,
  onApprovalSkillChangesChange,
}: {
  approvalNewAgents: boolean;
  approvalTaskCompletion: boolean;
  approvalSkillChanges: boolean;
  onApprovalNewAgentsChange: (v: boolean) => void;
  onApprovalTaskCompletionChange: (v: boolean) => void;
  onApprovalSkillChangesChange: (v: boolean) => void;
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
      <div>
        <SectionHeader>Appearance</SectionHeader>
        <AppearanceSection
          name={name}
          description={description}
          logoPreview={logoPreview}
          initial={initial}
          fileInputRef={fileInputRef}
          onNameChange={setName}
          onDescriptionChange={setDescription}
          onLogoChange={handleLogoChange}
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
          onApprovalNewAgentsChange={setApprovalNewAgents}
          onApprovalTaskCompletionChange={setApprovalTaskCompletion}
          onApprovalSkillChangesChange={setApprovalSkillChanges}
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

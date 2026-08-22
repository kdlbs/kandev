"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import type { TFunction } from "i18next";
import Image from "@/components/routing/app-image";
import { IconUpload, IconDeviceFloppy } from "@tabler/icons-react";
import { toast } from "@/lib/toast/sonner";
import { Input } from "@kandev/ui/input";
import { Switch } from "@kandev/ui/switch";
import { Button } from "@kandev/ui/button";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { updateWorkspaceSettings, getWorkspaceSettings } from "@/lib/api/domains/office-api";
import { updateWorkspaceAction } from "@/app/actions/workspaces";
import type { AppState } from "@/lib/state/store";
import type { WorkspaceState } from "@/lib/state/slices/workspace/types";
import { ConfigSection } from "./config-section";
import { DangerZoneSection } from "./danger-zone-section";
import { GitSection } from "./git-section";
import { useTranslation } from "react-i18next";

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
  const { t } = useTranslation();
  return (
    <SettingCard>
      <div className="flex items-center gap-4">
        <div className="h-14 w-14 rounded-xl bg-primary text-primary-foreground flex items-center justify-center text-lg font-semibold shrink-0 overflow-hidden">
          {logoPreview ? (
            <Image
              src={logoPreview}
              alt={t("office:logo")}
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
          <p className="text-sm text-muted-foreground mb-2">{t("office:logo")}</p>
          <Button
            variant="outline"
            size="sm"
            className="cursor-pointer"
            onClick={() => fileInputRef.current?.click()}
          >
            <IconUpload className="h-3.5 w-3.5 mr-1.5" />
            {t("office:uploadLogo")}
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
        <label className="text-sm text-muted-foreground">{t("office:name")}</label>
        <Input
          value={name}
          onChange={(e) => onNameChange(e.target.value)}
          placeholder={t("office:workspaceName")}
          className="mt-1"
        />
      </div>
      <div>
        <label className="text-sm text-muted-foreground">{t("office:description")}</label>
        <Input
          value={description}
          onChange={(e) => onDescriptionChange(e.target.value)}
          placeholder={t("office:optionalDescription")}
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
  const { t } = useTranslation();
  return (
    <SettingCard>
      <ToggleRow
        label={t("office:requireApprovalForNewAgents")}
        description={t("office:newAgentHiresMustBeApproved")}
        checked={approvalNewAgents}
        onCheckedChange={onApprovalNewAgentsChange}
      />
      <ToggleRow
        label={t("office:requireApprovalForTaskCompletion")}
        description={t("office:tasksMustBeReviewedBeforeThey")}
        checked={approvalTaskCompletion}
        onCheckedChange={onApprovalTaskCompletionChange}
      />
      <ToggleRow
        label={t("office:requireApprovalForSkillChanges")}
        description={t("office:agentCreatedSkillsMustBeApproved")}
        checked={approvalSkillChanges}
        onCheckedChange={onApprovalSkillChangesChange}
      />
      {dirty && (
        <div className="flex justify-end pt-2">
          <Button size="sm" onClick={onSave} disabled={saving} className="cursor-pointer">
            <IconDeviceFloppy className="h-4 w-4 mr-1.5" />
            {saving ? t("office:saving") : t("common:save")}
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
  const { t } = useTranslation();
  return (
    <SettingCard>
      <div className="flex items-center justify-between gap-4">
        <div>
          <p className="text-sm">{t("office:recoveryLookbackWindow")}</p>
          <p className="text-xs text-muted-foreground mt-0.5">{t("office:howFarBackToLookFor")}</p>
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
          <span className="text-sm text-muted-foreground">{t("office:hours")}</span>
        </div>
      </div>
      {dirty && (
        <div className="flex justify-end pt-2">
          <Button size="sm" onClick={onSave} disabled={saving} className="cursor-pointer">
            <IconDeviceFloppy className="h-4 w-4 mr-1.5" />
            {saving ? t("office:saving") : t("common:save")}
          </Button>
        </div>
      )}
    </SettingCard>
  );
}

type Workspace = WorkspaceState["items"][number];

type WorkspaceStoreApi = {
  getState: () => Pick<AppState, "workspaces" | "setWorkspaces">;
};

type SaveAppearanceOptions = {
  activeWorkspace: Workspace | undefined;
  name: string;
  description: string;
  storeApi: WorkspaceStoreApi;
  nameRef: React.RefObject<string>;
  descriptionRef: React.RefObject<string>;
  setName: (v: string) => void;
  setDescription: (v: string) => void;
  setSaving: (v: boolean) => void;
  t: TFunction;
};

function buildSaveAppearanceHandler({
  activeWorkspace,
  name,
  description,
  storeApi,
  nameRef,
  descriptionRef,
  setName,
  setDescription,
  setSaving,
  t,
}: SaveAppearanceOptions) {
  return async () => {
    if (!activeWorkspace) return;
    const trimmedName = name.trim();
    if (!trimmedName) {
      toast.error(t("workspaces:workspaceNameIsRequired"));
      return;
    }
    setSaving(true);
    try {
      const updated = await updateWorkspaceAction(activeWorkspace.id, {
        name: trimmedName,
        description,
      });
      // Only echo the server response back into the draft fields if the user
      // hasn't kept typing since this save started: the PATCH round trip can
      // take long enough for a newer keystroke to land before the response
      // does, and unconditionally overwriting the draft here would silently
      // discard it (refs, not the closed-over name/description, hold the
      // latest value across that round trip).
      if (nameRef.current === name) setName(updated.name);
      if (descriptionRef.current === description) setDescription(updated.description ?? "");
      // Read the store's current list at save time, not the array captured at
      // render/handler-build time: an in-flight workspace.created/updated/deleted
      // WS event can land during the PATCH round trip, and a stale snapshot here
      // would clobber it with a whole-array replace (setWorkspaces has no merge
      // semantics). Same idiom as config-chat-agent-section.tsx.
      const { workspaces: current, setWorkspaces } = storeApi.getState();
      setWorkspaces(
        current.items.map((ws) =>
          ws.id === updated.id
            ? { ...ws, name: updated.name, description: updated.description }
            : ws,
        ),
      );
      toast.success(t("office:appearanceSettingsSaved"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("office:failedToSaveSettings"));
      throw err;
    } finally {
      setSaving(false);
    }
  };
}

function usePermissionsState(activeWorkspace: Workspace | undefined) {
  const { t } = useTranslation();
  const [approvalNewAgents, setApprovalNewAgents] = useState(true);
  const [approvalTaskCompletion, setApprovalTaskCompletion] = useState(false);
  const [approvalSkillChanges, setApprovalSkillChanges] = useState(true);
  const [origApprovalNewAgents, setOrigApprovalNewAgents] = useState(true);
  const [origApprovalTaskCompletion, setOrigApprovalTaskCompletion] = useState(false);
  const [origApprovalSkillChanges, setOrigApprovalSkillChanges] = useState(true);
  const [savingPermissions, setSavingPermissions] = useState(false);
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
      toast.success(t("office:permissionSettingsSaved"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("office:failedToSaveSettings"));
    } finally {
      setSavingPermissions(false);
    }
  }, [activeWorkspace, approvalNewAgents, approvalTaskCompletion, approvalSkillChanges]);

  return {
    approvalNewAgents,
    setApprovalNewAgents,
    approvalTaskCompletion,
    setApprovalTaskCompletion,
    approvalSkillChanges,
    setApprovalSkillChanges,
    savingPermissions,
    permissionsDirty:
      approvalNewAgents !== origApprovalNewAgents ||
      approvalTaskCompletion !== origApprovalTaskCompletion ||
      approvalSkillChanges !== origApprovalSkillChanges,
    handleSavePermissions,
  };
}

function useRecoveryState(activeWorkspace: Workspace | undefined) {
  const { t } = useTranslation();
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
      toast.success(t("office:recoverySettingsSaved"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("office:failedToSaveSettings"));
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

type AppearanceBaseline = {
  id: string | undefined;
  name: string;
  description: string;
};

function getAppearanceBaseline(workspace: Workspace | undefined): AppearanceBaseline {
  return {
    id: workspace?.id,
    name: workspace?.name ?? "",
    description: workspace?.description ?? "",
  };
}

function reconcileAppearanceDraft(
  previous: AppearanceBaseline,
  next: AppearanceBaseline,
  setName: Dispatch<SetStateAction<string>>,
  setDescription: Dispatch<SetStateAction<string>>,
) {
  if (next.id !== previous.id) {
    setName(next.name);
    setDescription(next.description);
    return;
  }

  setName((current) => (current === previous.name ? next.name : current));
  setDescription((current) => (current === previous.description ? next.description : current));
}

export function useSettingsState(
  activeWorkspace: Workspace | undefined,
  storeApi: WorkspaceStoreApi,
) {
  const { t } = useTranslation();
  const appearance = getAppearanceBaseline(activeWorkspace);
  const [name, setName] = useState(appearance.name);
  const [description, setDescription] = useState(appearance.description);
  const [logoPreview, setLogoPreview] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [savingAppearance, setSavingAppearance] = useState(false);
  const recovery = useRecoveryState(activeWorkspace);
  const permissions = usePermissionsState(activeWorkspace);
  const { id: appearanceId, name: appearanceName, description: appearanceDescription } = appearance;
  const appearanceBaselineRef = useRef(appearance);

  // If the active workspace's identity changes without this page
  // remounting - e.g. a workspace.deleted WS event elsewhere reassigns
  // workspaces.activeId while this page stays mounted - the draft must
  // reset to the new workspace's values. Otherwise the stale draft from
  // the old workspace looks "dirty" against the new one's name, and Save
  // would persist the old workspace's data onto the new, unrelated one.
  // For same-workspace updates, only fields that still match the previous
  // baseline are refreshed. This keeps remote changes visible without
  // overwriting a local draft.
  useEffect(() => {
    const next = {
      id: appearanceId,
      name: appearanceName,
      description: appearanceDescription,
    };
    const previous = appearanceBaselineRef.current;

    reconcileAppearanceDraft(previous, next, setName, setDescription);
    appearanceBaselineRef.current = next;
  }, [appearanceDescription, appearanceId, appearanceName]);

  // Latest-value refs so the in-flight save handler (a closure fixed at the
  // moment Save was clicked) can tell whether the draft has moved on since,
  // instead of blindly trusting the name/description it captured at submit
  // time.
  const nameRef = useRef(name);
  const descriptionRef = useRef(description);
  useEffect(() => {
    nameRef.current = name;
  }, [name]);
  useEffect(() => {
    descriptionRef.current = description;
  }, [description]);

  const handleLogoChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) setLogoPreview(URL.createObjectURL(file));
  };

  // useCallback (matching handleSavePermissions/handleSaveRecovery) so the
  // handler only gets a new identity when an input it actually reads changes.
  const handleSaveAppearance = useCallback(
    buildSaveAppearanceHandler({
      activeWorkspace,
      name,
      description,
      storeApi,
      nameRef,
      descriptionRef,
      setName,
      setDescription,
      setSaving: setSavingAppearance,
      t,
    }),
    [activeWorkspace, name, description, storeApi, nameRef, descriptionRef, t],
  );

  const appearanceDirty =
    Boolean(appearanceId) && (name !== appearanceName || description !== appearanceDescription);
  const appearanceRevision = JSON.stringify({
    id: appearanceId,
    name,
    description,
  });

  useSettingsSaveContributor({
    id: "office-workspace-appearance",
    order: 10,
    revision: appearanceRevision,
    isDirty: appearanceDirty,
    canSave: Boolean(name.trim()),
    invalidReason: name.trim() ? undefined : t("workspaces:workspaceNameIsRequired"),
    save: handleSaveAppearance,
    discard: (revision) => {
      if (!Object.is(revision, appearanceRevision)) return;
      setName(appearanceName);
      setDescription(appearanceDescription);
    },
  });

  return {
    name,
    setName,
    description,
    setDescription,
    logoPreview,
    fileInputRef,
    ...permissions,
    ...recovery,
    appearanceDirty,
    savingAppearance,
    handleLogoChange,
    handleSaveAppearance,
  };
}

export function SettingsContent() {
  const { t } = useTranslation();
  const storeApi = useAppStoreApi();
  const workspaces = useAppStore((s) => s.workspaces);
  const setWorkspaces = useAppStore((s) => s.setWorkspaces);
  const setActiveWorkspace = useAppStore((s) => s.setActiveWorkspace);
  const activeWorkspace = workspaces.items.find((w) => w.id === workspaces.activeId);
  const s = useSettingsState(activeWorkspace, storeApi);
  const initial = (s.name || "W").charAt(0).toUpperCase();

  return (
    <div className="max-w-3xl mx-auto p-6 space-y-8">
      <div>
        <SectionHeader>{t("office:appearance")}</SectionHeader>
        <AppearanceSection
          name={s.name}
          description={s.description}
          logoPreview={s.logoPreview}
          initial={initial}
          fileInputRef={s.fileInputRef}
          onNameChange={s.setName}
          onDescriptionChange={s.setDescription}
          onLogoChange={s.handleLogoChange}
        />
      </div>

      <div>
        <SectionHeader>{t("common:repository")}</SectionHeader>
        <SettingCard>
          <GitSection />
        </SettingCard>
      </div>

      <div>
        <SectionHeader>{t("office:permissions")}</SectionHeader>
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
        <SectionHeader>{t("office:recovery")}</SectionHeader>
        <RecoverySection
          lookbackHours={s.lookbackHours}
          dirty={s.recoveryDirty}
          saving={s.savingRecovery}
          onLookbackChange={s.setLookbackHours}
          onSave={s.handleSaveRecovery}
        />
      </div>

      <div>
        <SectionHeader>{t("office:configuration")}</SectionHeader>
        <SettingCard>
          <ConfigSection />
        </SettingCard>
      </div>

      {activeWorkspace && (
        <div>
          <SectionHeader>{t("office:dangerZone")}</SectionHeader>
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

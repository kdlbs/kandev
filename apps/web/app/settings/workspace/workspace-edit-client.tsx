"use client";

import { useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { useRouter } from "@/lib/routing/client-router";
import { runWithNavigationBlockerBypassed } from "@/lib/routing/navigation-guard";
import { IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { updateWorkspaceAction, deleteWorkspaceAction } from "@/app/actions/workspaces";
import type { Executor } from "@/lib/types/http";
import type { AgentProfileOption, WorkspaceState } from "@/lib/state/slices";
import { isSelectableAgentProfile } from "@/lib/state/slices/settings/types";
import { buildWorkspaceSaveHandler, type SavedState } from "./workspace-edit-save";
import { useWorkspaceIdlePolicyDraft } from "./workspace-edit-idle-policy-draft";

type Workspace = WorkspaceState["items"][number];
import { useRequest } from "@/lib/http/use-request";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { SettingsGroup } from "@/components/settings/settings-group";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { SettingsTarget } from "@/components/settings/settings-target";
import { workspaceDiscoveryTarget } from "@/lib/settings-discovery/dynamic-targets";
import { WorkspaceSectionHeader } from "@/components/settings/workspaces/workspace-section-header";
import { WorkspacePlacementCard } from "@/components/settings/workspaces/workspace-placement-card";
import { WorkspaceTeamAccessCard } from "@/components/settings/workspaces/workspace-team-access-card";
import { hasScope, SCOPE } from "@/lib/types/team-access";
import { WorkspaceIdlePolicyFormSection } from "./workspace-idle-policy-form-section";

type WorkspaceEditClientProps = {
  workspaceId: string;
};

export function WorkspaceEditClient({ workspaceId }: WorkspaceEditClientProps) {
  const workspace = useAppStore(
    (state) => state.workspaces.items.find((item: Workspace) => item.id === workspaceId) ?? null,
  );
  const { t } = useTranslation();

  if (!workspace) {
    return (
      <div>
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-muted-foreground">{t("workspaces:workspaceNotFound")}</p>
            <Button className="mt-4" asChild>
              <Link href="/settings/workspaces">{t("workspaces:backToWorkspaces")}</Link>
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return <WorkspaceEditForm key={workspace.id} workspace={workspace} />;
}

type WorkspaceEditFormProps = {
  workspace: Workspace;
};

type SelectFieldProps = {
  label: string;
  // The placeholder was `Select ${label.toLowerCase()}`. Lowercasing a translated
  // label is an English-only transformation, and building the sentence at the
  // call site leaves a translator with no way to reorder it — so it is its own
  // message, passed in by the caller.
  placeholder: string;
  value: string;
  isDirty: boolean;
  onChange: (v: string) => void;
  options: { id: string; name: string }[];
  emptyLabel: string;
  emptyValue: string;
  discoveryTargetId: string;
};

function SelectField({
  label,
  placeholder,
  value,
  isDirty,
  onChange,
  options,
  emptyLabel,
  emptyValue,
  discoveryTargetId,
}: SelectFieldProps) {
  const { t } = useTranslation();
  return (
    <SettingsTarget targetId={discoveryTargetId} className="space-y-2">
      <Label>{label}</Label>
      <Select value={value || "none"} onValueChange={(v) => onChange(v === "none" ? "" : v)}>
        <SelectTrigger className="w-full" data-settings-dirty={isDirty}>
          <SelectValue placeholder={placeholder} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="none">{t("workspaces:noDefault")}</SelectItem>
          {options.map((opt) => (
            <SelectItem key={opt.id} value={opt.id}>
              {opt.name}
            </SelectItem>
          ))}
          {options.length === 0 && (
            <SelectItem value={emptyValue} disabled>
              {emptyLabel}
            </SelectItem>
          )}
        </SelectContent>
      </Select>
    </SettingsTarget>
  );
}

type WorkspaceSettingsCardProps = {
  canManage: boolean;
  workspaceId: string;
  workspaceNameDraft: string;
  nameIsDirty: boolean;
  onNameChange: (value: string) => void;
  defaultExecutorId: string;
  executorIsDirty: boolean;
  onExecutorChange: (value: string) => void;
  activeExecutors: Executor[];
  executorsEmpty: boolean;
  defaultAgentProfileId: string;
  agentProfileIsDirty: boolean;
  onAgentProfileChange: (value: string) => void;
  agentProfiles: AgentProfileOption[];
};

function WorkspaceSettingsCard({
  canManage,
  workspaceId,
  workspaceNameDraft,
  nameIsDirty,
  onNameChange,
  defaultExecutorId,
  executorIsDirty,
  onExecutorChange,
  activeExecutors,
  executorsEmpty,
  defaultAgentProfileId,
  agentProfileIsDirty,
  onAgentProfileChange,
  agentProfiles,
}: WorkspaceSettingsCardProps) {
  const { t } = useTranslation();
  const dynamicRoutingEnabled = useFeature("dynamicAgentRouting");
  const executorOptions = activeExecutors.map((e: Executor) => ({ id: e.id, name: e.name }));
  const profileOptions = agentProfiles
    .filter((profile) => isSelectableAgentProfile(profile, dynamicRoutingEnabled))
    .map((p: AgentProfileOption) => ({
      id: p.id,
      name: p.label,
    }));
  return (
    <SettingsGroup
      title={t("workspaces:workspaceSettings")}
      isDirty={nameIsDirty || executorIsDirty || agentProfileIsDirty}
      contentClassName="space-y-4 divide-y-0"
    >
      <div className="space-y-4">
        <SettingsTarget
          targetId={workspaceDiscoveryTarget(workspaceId, "name")}
          className="space-y-2"
        >
          <Label htmlFor="workspace-name">{t("workspaces:name")}</Label>
          <Input
            id="workspace-name"
            value={workspaceNameDraft}
            disabled={!canManage}
            data-settings-dirty={nameIsDirty}
            onChange={(e) => onNameChange(e.target.value)}
          />
        </SettingsTarget>
        <SelectField
          label={t("workspaces:defaultExecutor")}
          placeholder={t("workspaces:selectDefaultExecutor")}
          value={defaultExecutorId}
          isDirty={executorIsDirty}
          onChange={onExecutorChange}
          options={executorsEmpty ? [] : executorOptions}
          emptyLabel={t("workspaces:noExecutorsAvailable")}
          emptyValue=""
          discoveryTargetId={workspaceDiscoveryTarget(workspaceId, "default-executor")}
        />
        <SelectField
          label={t("workspaces:defaultAgentProfile")}
          placeholder={t("workspaces:selectDefaultAgentProfile")}
          value={defaultAgentProfileId}
          isDirty={agentProfileIsDirty}
          onChange={onAgentProfileChange}
          options={profileOptions}
          emptyLabel={t("workspaces:noAgentProfilesAvailable")}
          emptyValue="empty-agent-profiles"
          discoveryTargetId={workspaceDiscoveryTarget(workspaceId, "default-agent-profile")}
        />
      </div>
    </SettingsGroup>
  );
}

type DeleteWorkspaceCardProps = {
  canManage: boolean;
  workspaceName: string;
  deleteDialogOpen: boolean;
  setDeleteDialogOpen: (open: boolean) => void;
  deleteConfirmText: string;
  setDeleteConfirmText: (text: string) => void;
  onDelete: () => void;
};

function DeleteWorkspaceCard({
  canManage,
  workspaceName,
  deleteDialogOpen,
  setDeleteDialogOpen,
  deleteConfirmText,
  setDeleteConfirmText,
  onDelete,
}: DeleteWorkspaceCardProps) {
  const { t } = useTranslation();
  // Deleting a workspace is owner-only. The API refuses a member either way;
  // hiding the card keeps the UI from offering an action it will reject.
  if (!canManage) return null;
  return (
    <>
      <Card className="border-destructive">
        <CardHeader>
          <CardTitle className="text-destructive">{t("workspaces:deleteWorkspace")}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">{t("workspaces:deleteThisWorkspace")}</p>
              <p className="text-xs text-muted-foreground">
                {t("workspaces:thisActionCannotBeUndone")}
              </p>
            </div>
            <Button
              variant="destructive"
              onClick={() => setDeleteDialogOpen(true)}
              className="cursor-pointer"
              data-testid="workspace-settings-delete-button"
            >
              <IconTrash className="h-4 w-4 mr-2" />
              {t("workspaces:delete")}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("workspaces:deleteWorkspace")}</DialogTitle>
            <DialogDescription>
              {/* The workspace name is user data and is what the input is compared
                  against, so it travels as an interpolated value — never as part
                  of the message a translator edits. */}
              <Trans
                i18nKey="workspaces:typeWorkspaceNameToConfirm"
                values={{ name: workspaceName }}
              >
                <span className="font-medium" />
              </Trans>
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="confirm-delete">{t("workspaces:confirmDelete")}</Label>
            <Input
              id="confirm-delete"
              value={deleteConfirmText}
              onChange={(event) => setDeleteConfirmText(event.target.value)}
              placeholder={workspaceName}
              autoComplete="off"
              data-testid="workspace-settings-delete-confirm-input"
            />
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setDeleteDialogOpen(false)}
              className="cursor-pointer"
            >
              {t("common:cancel")}
            </Button>
            <Button
              variant="destructive"
              onClick={onDelete}
              disabled={deleteConfirmText !== workspaceName}
              className="cursor-pointer"
              data-testid="workspace-settings-delete-confirm-button"
            >
              {t("workspaces:delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function useWorkspaceDeleteDraft(
  workspace: Workspace,
  workspaces: Workspace[],
  setWorkspaces: (items: Workspace[]) => void,
) {
  const router = useRouter();
  const { toast } = useToast();
  const { t } = useTranslation();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [confirmText, setConfirmText] = useState("");
  const deleteRequest = useRequest(deleteWorkspaceAction);
  const officeEnabled = useFeature("office");

  const handleDelete = async () => {
    if (confirmText !== workspace.name) return;
    try {
      await deleteRequest.run(workspace.id, workspace.name, officeEnabled);
      setWorkspaces(workspaces.filter((item) => item.id !== workspace.id));
      runWithNavigationBlockerBypassed(() => router.push("/settings/workspaces"));
    } catch (error) {
      toast({
        title: t("workspaces:failedToDeleteWorkspace"),
        description: error instanceof Error ? error.message : t("common:requestFailed"),
        variant: "error",
      });
    }
  };

  const handleDialogOpenChange = (open: boolean) => {
    setDialogOpen(open);
    if (!open) setConfirmText("");
  };

  return {
    deleteDialogOpen: dialogOpen,
    setDeleteDialogOpen: handleDialogOpenChange,
    deleteConfirmText: confirmText,
    setDeleteConfirmText: setConfirmText,
    handleDeleteWorkspace: handleDelete,
  };
}

function useWorkspaceEditForm(workspace: Workspace) {
  const { toast } = useToast();
  const { t } = useTranslation();
  const [currentWorkspace, setCurrentWorkspace] = useState<Workspace>(workspace);
  const [workspaceNameDraft, setWorkspaceNameDraft] = useState(workspace.name ?? "");
  const [defaultExecutorId, setDefaultExecutorId] = useState(workspace.default_executor_id ?? "");
  const [defaultAgentProfileId, setDefaultAgentProfileId] = useState(
    workspace.default_agent_profile_id ?? "",
  );
  const [savedState, setSavedState] = useState<SavedState>({
    name: workspace.name ?? "",
    executorId: workspace.default_executor_id ?? "",
    agentProfileId: workspace.default_agent_profile_id ?? "",
    idleSuspensionEnabled: workspace.acp_idle_suspension_enabled ?? false,
    idleTimeoutMinutes: workspace.acp_idle_timeout_minutes ?? 120,
  });
  const {
    enabled: idleSuspensionEnabled,
    setEnabled: setIdleSuspensionEnabled,
    timeoutMinutes: idleTimeoutMinutes,
    setTimeoutMinutes: setIdleTimeoutMinutes,
    timeoutValid: idleTimeoutValid,
    isDirty: idlePolicyIsDirty,
  } = useWorkspaceIdlePolicyDraft(workspace, savedState);
  const executors = useAppStore((state) => state.executors.items);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const workspaces = useAppStore((state) => state.workspaces.items);
  const setWorkspaces = useAppStore((state) => state.setWorkspaces);
  const deleteDraft = useWorkspaceDeleteDraft(currentWorkspace, workspaces, setWorkspaces);

  const saveWorkspaceRequest = useRequest(updateWorkspaceAction);

  const activeExecutors = executors.filter((executor: Executor) => executor.status === "active");
  const isDirty =
    workspaceNameDraft.trim() !== savedState.name ||
    defaultExecutorId !== savedState.executorId ||
    defaultAgentProfileId !== savedState.agentProfileId;
  const isWorkspaceDirty = isDirty || idlePolicyIsDirty;

  const handleSave = buildWorkspaceSaveHandler({
    currentWorkspace,
    draft: {
      workspaceNameDraft,
      defaultExecutorId,
      defaultAgentProfileId,
      idleSuspensionEnabled,
      idleTimeoutMinutes,
    },
    savedState,
    isDirty: isWorkspaceDirty,
    setSavedState,
    setCurrentWorkspace,
    workspaces,
    setWorkspaces,
    saveWorkspaceRequest,
    toast,
    t,
  });

  const handleDiscard = () => {
    setWorkspaceNameDraft(savedState.name);
    setDefaultExecutorId(savedState.executorId);
    setDefaultAgentProfileId(savedState.agentProfileId);
    setIdleSuspensionEnabled(savedState.idleSuspensionEnabled);
    setIdleTimeoutMinutes(String(savedState.idleTimeoutMinutes));
  };

  return {
    currentWorkspace,
    workspaceNameDraft,
    setWorkspaceNameDraft,
    defaultExecutorId,
    setDefaultExecutorId,
    defaultAgentProfileId,
    setDefaultAgentProfileId,
    idleSuspensionEnabled,
    setIdleSuspensionEnabled,
    idleTimeoutMinutes,
    setIdleTimeoutMinutes,
    activeExecutors,
    executors,
    agentProfiles,
    savedState,
    isDirty: isWorkspaceDirty,
    idleTimeoutValid,
    idlePolicyIsDirty,
    handleSave,
    handleDiscard,
    ...deleteDraft,
  };
}

type WorkspaceFormSaveOptions = {
  workspaceId: string;
  workspaceNameDraft: string;
  defaultExecutorId: string;
  defaultAgentProfileId: string;
  idleSuspensionEnabled: boolean;
  idleTimeoutMinutes: string;
  idleTimeoutValid: boolean;
  isDirty: boolean;
  save: () => Promise<void>;
  discard: () => void;
};

function useWorkspaceFormSaveContributor(options: WorkspaceFormSaveOptions) {
  const { t } = useTranslation();
  const hasWorkspaceName = Boolean(options.workspaceNameDraft.trim());
  const invalidReason = workspaceDraftInvalidReason(hasWorkspaceName, options.idleTimeoutValid, t);
  useSettingsSaveContributor({
    id: `workspace:${options.workspaceId}`,
    revision: JSON.stringify({
      workspaceNameDraft: options.workspaceNameDraft,
      defaultExecutorId: options.defaultExecutorId,
      defaultAgentProfileId: options.defaultAgentProfileId,
      idleSuspensionEnabled: options.idleSuspensionEnabled,
      idleTimeoutMinutes: options.idleTimeoutMinutes,
    }),
    isDirty: options.isDirty,
    canSave: hasWorkspaceName && options.idleTimeoutValid,
    invalidReason,
    save: options.save,
    discard: options.discard,
  });
}

function workspaceDraftInvalidReason(
  hasWorkspaceName: boolean,
  timeoutValid: boolean,
  t: ReturnType<typeof useTranslation>["t"],
) {
  if (!hasWorkspaceName) return t("workspaces:workspaceNameIsRequired");
  if (!timeoutValid) return t("workspaces:idleTimeoutMustBePositive");
  return undefined;
}

function WorkspaceEditForm({ workspace }: WorkspaceEditFormProps) {
  const {
    currentWorkspace,
    workspaceNameDraft,
    setWorkspaceNameDraft,
    defaultExecutorId,
    setDefaultExecutorId,
    defaultAgentProfileId,
    setDefaultAgentProfileId,
    idleSuspensionEnabled,
    setIdleSuspensionEnabled,
    idleTimeoutMinutes,
    setIdleTimeoutMinutes,
    deleteDialogOpen,
    setDeleteDialogOpen,
    deleteConfirmText,
    setDeleteConfirmText,
    activeExecutors,
    executors,
    agentProfiles,
    savedState,
    isDirty,
    idleTimeoutValid,
    idlePolicyIsDirty,
    handleSave,
    handleDiscard,
    handleDeleteWorkspace,
  } = useWorkspaceEditForm(workspace);
  // Owner-only controls are gated on the server-issued scope, never on an
  // owner-id comparison, so the UI and the API agree on one answer.
  const canManage = hasScope(workspace.scopes, SCOPE.workspaceManage);
  const { t } = useTranslation();

  useWorkspaceFormSaveContributor({
    workspaceId: currentWorkspace.id,
    workspaceNameDraft,
    defaultExecutorId,
    defaultAgentProfileId,
    idleSuspensionEnabled,
    idleTimeoutMinutes,
    idleTimeoutValid,
    isDirty,
    save: handleSave,
    discard: handleDiscard,
  });

  return (
    <div className="space-y-8">
      <WorkspaceSectionHeader tab="overview" description={t("workspaces:manageWorkspaceDetails")} />
      <Separator />
      <WorkspaceSettingsCard
        canManage={canManage}
        workspaceId={currentWorkspace.id}
        workspaceNameDraft={workspaceNameDraft}
        nameIsDirty={workspaceNameDraft.trim() !== savedState.name}
        onNameChange={setWorkspaceNameDraft}
        defaultExecutorId={defaultExecutorId}
        executorIsDirty={defaultExecutorId !== savedState.executorId}
        onExecutorChange={setDefaultExecutorId}
        activeExecutors={activeExecutors}
        executorsEmpty={executors.length === 0}
        defaultAgentProfileId={defaultAgentProfileId}
        agentProfileIsDirty={defaultAgentProfileId !== savedState.agentProfileId}
        onAgentProfileChange={setDefaultAgentProfileId}
        agentProfiles={agentProfiles}
      />
      <WorkspaceIdlePolicyFormSection
        canManage={canManage}
        enabled={idleSuspensionEnabled}
        timeoutMinutes={idleTimeoutMinutes}
        timeoutValid={idleTimeoutValid}
        savedTimeoutMinutes={savedState.idleTimeoutMinutes}
        enabledIsDirty={idleSuspensionEnabled !== savedState.idleSuspensionEnabled}
        timeoutIsDirty={idlePolicyIsDirty}
        onEnabledChange={setIdleSuspensionEnabled}
        onTimeoutChange={setIdleTimeoutMinutes}
      />
      {/*
        Reads the live store item rather than the form's captured snapshot:
        scopes arrive with hydration, which can land after this
        form mounts, and a snapshot would freeze the owner out of their own
        workspace.
      */}
      <WorkspacePlacementCard
        workspaceId={workspace.id}
        unitId={workspace.unit_id}
        scopes={workspace.scopes}
      />
      <Separator />
      <WorkspaceTeamAccessCard
        workspaceId={workspace.id}
        ownerId={workspace.owner_id ?? ""}
        scopes={workspace.scopes}
      />
      <Separator />
      <DeleteWorkspaceCard
        canManage={canManage}
        workspaceName={currentWorkspace.name}
        deleteDialogOpen={deleteDialogOpen}
        setDeleteDialogOpen={setDeleteDialogOpen}
        deleteConfirmText={deleteConfirmText}
        setDeleteConfirmText={setDeleteConfirmText}
        onDelete={handleDeleteWorkspace}
      />
    </div>
  );
}

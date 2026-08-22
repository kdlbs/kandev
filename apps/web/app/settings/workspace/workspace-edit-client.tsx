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
import type { TFunction } from "i18next";
import type { Executor } from "@/lib/types/http";
import type { AgentProfileOption, WorkspaceState } from "@/lib/state/slices";
import { isSelectableAgentProfile } from "@/lib/state/slices/settings/types";

type Workspace = WorkspaceState["items"][number];
import { useRequest } from "@/lib/http/use-request";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { SettingsCard } from "@/components/settings/settings-card";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { SettingsTarget } from "@/components/settings/settings-target";
import { workspaceDiscoveryTarget } from "@/lib/settings-discovery/dynamic-targets";
import { WorkspaceSectionHeader } from "@/components/settings/workspaces/workspace-section-header";
import { WorkspaceTeamAccessCard } from "@/components/settings/workspaces/workspace-team-access-card";
import { hasScope, SCOPE, type WorkspaceVisibility } from "@/lib/types/team-access";

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
    <SettingsCard isDirty={nameIsDirty || executorIsDirty || agentProfileIsDirty}>
      <CardHeader>
        <CardTitle>{t("workspaces:workspaceSettings")}</CardTitle>
      </CardHeader>
      <CardContent>
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
      </CardContent>
    </SettingsCard>
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

type SavedState = {
  name: string;
  executorId: string;
  agentProfileId: string;
};

function buildWorkspaceUpdates(
  draft: { name: string; executorId: string; agentProfileId: string },
  saved: SavedState,
): Record<string, string | undefined> {
  const updates: Record<string, string | undefined> = {};
  if (draft.name.trim() !== saved.name) updates.name = draft.name.trim();
  if (draft.executorId !== saved.executorId) updates.default_executor_id = draft.executorId;
  if (draft.agentProfileId !== saved.agentProfileId)
    updates.default_agent_profile_id = draft.agentProfileId;
  return updates;
}

type WorkspaceDraftState = {
  workspaceNameDraft: string;
  defaultExecutorId: string;
  defaultAgentProfileId: string;
};

type SaveRequestLike = {
  run: (id: string, updates: Record<string, string | undefined>) => Promise<Workspace>;
};

type WorkspaceSaveHandlerOptions = {
  currentWorkspace: Workspace;
  draft: WorkspaceDraftState;
  savedState: SavedState;
  isDirty: boolean;
  setSavedState: (s: SavedState) => void;
  setCurrentWorkspace: (fn: (prev: Workspace) => Workspace) => void;
  workspaces: Workspace[];
  setWorkspaces: (items: Workspace[]) => void;
  saveWorkspaceRequest: SaveRequestLike;
  toast: ReturnType<typeof useToast>["toast"];
  t: TFunction;
};

function buildSaveHandler({
  currentWorkspace,
  draft,
  savedState,
  isDirty,
  setSavedState,
  setCurrentWorkspace,
  workspaces,
  setWorkspaces,
  saveWorkspaceRequest,
  toast,
  t,
}: WorkspaceSaveHandlerOptions) {
  return async () => {
    if (!isDirty) return;
    try {
      const updates = buildWorkspaceUpdates(
        {
          name: draft.workspaceNameDraft,
          executorId: draft.defaultExecutorId,
          agentProfileId: draft.defaultAgentProfileId,
        },
        savedState,
      );
      const updated = await saveWorkspaceRequest.run(currentWorkspace.id, updates);
      setCurrentWorkspace((prev) => ({ ...prev, ...updated }));
      setSavedState({
        name: updated.name ?? draft.workspaceNameDraft.trim(),
        executorId: updated.default_executor_id ?? "",
        agentProfileId: updated.default_agent_profile_id ?? "",
      });
      setWorkspaces(
        workspaces.map((ws: Workspace) =>
          ws.id === updated.id
            ? {
                ...ws,
                name: updated.name,
                default_executor_id: updated.default_executor_id ?? null,
                default_environment_id: updated.default_environment_id ?? null,
                default_agent_profile_id: updated.default_agent_profile_id ?? null,
              }
            : ws,
        ),
      );
    } catch (error) {
      toast({
        title: t("workspaces:failedToSaveWorkspace"),
        description: error instanceof Error ? error.message : t("common:requestFailed"),
        variant: "error",
      });
      throw error;
    }
  };
}

function useWorkspaceEditForm(workspace: Workspace) {
  const router = useRouter();
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
  });
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState("");

  const executors = useAppStore((state) => state.executors.items);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const workspaces = useAppStore((state) => state.workspaces.items);
  const setWorkspaces = useAppStore((state) => state.setWorkspaces);

  const saveWorkspaceRequest = useRequest(updateWorkspaceAction);
  const deleteWorkspaceRequest = useRequest(deleteWorkspaceAction);
  // Selects the delete route: the office endpoint only exists while the feature
  // is on. See `deleteWorkspaceAction`.
  const officeEnabled = useFeature("office");

  const activeExecutors = executors.filter((executor: Executor) => executor.status === "active");
  const isDirty =
    workspaceNameDraft.trim() !== savedState.name ||
    defaultExecutorId !== savedState.executorId ||
    defaultAgentProfileId !== savedState.agentProfileId;

  const handleSave = buildSaveHandler({
    currentWorkspace,
    draft: { workspaceNameDraft, defaultExecutorId, defaultAgentProfileId },
    savedState,
    isDirty,
    setSavedState,
    setCurrentWorkspace,
    workspaces,
    setWorkspaces,
    saveWorkspaceRequest,
    toast,
    t,
  });

  const handleDeleteWorkspace = async () => {
    if (deleteConfirmText !== currentWorkspace.name) return;
    try {
      await deleteWorkspaceRequest.run(currentWorkspace.id, currentWorkspace.name, officeEnabled);
      setWorkspaces(workspaces.filter((ws: Workspace) => ws.id !== currentWorkspace.id));
      runWithNavigationBlockerBypassed(() => router.push("/settings/workspaces"));
    } catch (error) {
      toast({
        title: t("workspaces:failedToDeleteWorkspace"),
        description: error instanceof Error ? error.message : t("common:requestFailed"),
        variant: "error",
      });
    }
  };

  // Clears pre-fill so Cancel-then-reopen can't silently bypass the re-type requirement.
  const handleDeleteDialogOpenChange = (open: boolean) => {
    setDeleteDialogOpen(open);
    if (!open) setDeleteConfirmText("");
  };

  const handleDiscard = () => {
    setWorkspaceNameDraft(savedState.name);
    setDefaultExecutorId(savedState.executorId);
    setDefaultAgentProfileId(savedState.agentProfileId);
  };

  return {
    currentWorkspace,
    workspaceNameDraft,
    setWorkspaceNameDraft,
    defaultExecutorId,
    setDefaultExecutorId,
    defaultAgentProfileId,
    setDefaultAgentProfileId,
    deleteDialogOpen,
    setDeleteDialogOpen: handleDeleteDialogOpenChange,
    deleteConfirmText,
    setDeleteConfirmText,
    activeExecutors,
    executors,
    agentProfiles,
    savedState,
    isDirty,
    handleSave,
    handleDiscard,
    handleDeleteWorkspace,
  };
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
    deleteDialogOpen,
    setDeleteDialogOpen,
    deleteConfirmText,
    setDeleteConfirmText,
    activeExecutors,
    executors,
    agentProfiles,
    savedState,
    isDirty,
    handleSave,
    handleDiscard,
    handleDeleteWorkspace,
  } = useWorkspaceEditForm(workspace);
  // Owner-only controls are gated on the server-issued scope, never on an
  // owner-id comparison, so the UI and the API agree on one answer.
  const canManage = hasScope(workspace.scopes, SCOPE.workspaceManage);
  const { t } = useTranslation();

  useSettingsSaveContributor({
    id: `workspace:${currentWorkspace.id}`,
    revision: JSON.stringify({
      workspaceNameDraft,
      defaultExecutorId,
      defaultAgentProfileId,
    }),
    isDirty,
    canSave: Boolean(workspaceNameDraft.trim()),
    invalidReason: workspaceNameDraft.trim() ? undefined : t("workspaces:workspaceNameIsRequired"),
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
      <Separator />
      {/*
        Reads the live store item rather than the form's captured snapshot:
        scopes and visibility arrive with hydration, which can land after this
        form mounts, and a snapshot would freeze the owner out of their own
        workspace.
      */}
      <WorkspaceTeamAccessCard
        workspaceId={workspace.id}
        ownerId={workspace.owner_id ?? ""}
        visibility={(workspace.visibility as WorkspaceVisibility) ?? "private"}
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

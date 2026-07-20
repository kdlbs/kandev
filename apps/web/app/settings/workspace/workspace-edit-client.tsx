"use client";

import { useState } from "react";
import Link from "@/components/routing/app-link";
import { useRouter } from "@/lib/routing/client-router";
import { runWithNavigationBlockerBypassed } from "@/lib/routing/navigation-guard";
import { IconGitBranch, IconLayoutColumns, IconTrash } from "@tabler/icons-react";
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

type Workspace = WorkspaceState["items"][number];
import { useRequest } from "@/lib/http/use-request";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import { SettingsCard } from "@/components/settings/settings-card";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";

type WorkspaceEditClientProps = {
  workspaceId: string;
};

export function WorkspaceEditClient({ workspaceId }: WorkspaceEditClientProps) {
  const workspace = useAppStore(
    (state) => state.workspaces.items.find((item: Workspace) => item.id === workspaceId) ?? null,
  );

  if (!workspace) {
    return (
      <div>
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-muted-foreground">Workspace not found</p>
            <Button className="mt-4" asChild>
              <Link href="/settings/workspace">Back to Workspaces</Link>
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
  value: string;
  isDirty: boolean;
  onChange: (v: string) => void;
  options: { id: string; name: string }[];
  emptyLabel: string;
  emptyValue: string;
};

function SelectField({
  label,
  value,
  isDirty,
  onChange,
  options,
  emptyLabel,
  emptyValue,
}: SelectFieldProps) {
  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      <Select value={value || "none"} onValueChange={(v) => onChange(v === "none" ? "" : v)}>
        <SelectTrigger className="w-full" data-settings-dirty={isDirty}>
          <SelectValue placeholder={`Select ${label.toLowerCase()}`} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="none">No default</SelectItem>
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
    </div>
  );
}

type WorkspaceSettingsCardProps = {
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
  const executorOptions = activeExecutors.map((e: Executor) => ({ id: e.id, name: e.name }));
  const profileOptions = agentProfiles.map((p: AgentProfileOption) => ({
    id: p.id,
    name: p.label,
  }));
  return (
    <SettingsCard isDirty={nameIsDirty || executorIsDirty || agentProfileIsDirty}>
      <CardHeader>
        <CardTitle>Workspace Settings</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="workspace-name">Name</Label>
            <Input
              id="workspace-name"
              value={workspaceNameDraft}
              data-settings-dirty={nameIsDirty}
              onChange={(e) => onNameChange(e.target.value)}
            />
          </div>
          <SelectField
            label="Default Executor"
            value={defaultExecutorId}
            isDirty={executorIsDirty}
            onChange={onExecutorChange}
            options={executorsEmpty ? [] : executorOptions}
            emptyLabel="No executors available"
            emptyValue=""
          />
          <SelectField
            label="Default Agent Profile"
            value={defaultAgentProfileId}
            isDirty={agentProfileIsDirty}
            onChange={onAgentProfileChange}
            options={profileOptions}
            emptyLabel="No agent profiles available"
            emptyValue="empty-agent-profiles"
          />
        </div>
      </CardContent>
    </SettingsCard>
  );
}

type WorkspaceLinksCardProps = {
  workspaceId: string;
};

function WorkspaceLinksCard({ workspaceId }: WorkspaceLinksCardProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Workspace Links</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="grid gap-3 sm:grid-cols-2">
          <Button asChild variant="outline" className="justify-start gap-2">
            <Link href={`/settings/workspace/${workspaceId}/repositories`}>
              <IconGitBranch className="h-4 w-4" />
              Repositories
            </Link>
          </Button>
          <Button asChild variant="outline" className="justify-start gap-2">
            <Link href={`/settings/workspace/${workspaceId}/workflows`}>
              <IconLayoutColumns className="h-4 w-4" />
              Workflows
            </Link>
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

type DeleteWorkspaceCardProps = {
  workspaceName: string;
  deleteDialogOpen: boolean;
  setDeleteDialogOpen: (open: boolean) => void;
  deleteConfirmText: string;
  setDeleteConfirmText: (text: string) => void;
  onDelete: () => void;
};

function DeleteWorkspaceCard({
  workspaceName,
  deleteDialogOpen,
  setDeleteDialogOpen,
  deleteConfirmText,
  setDeleteConfirmText,
  onDelete,
}: DeleteWorkspaceCardProps) {
  return (
    <>
      <Card className="border-destructive">
        <CardHeader>
          <CardTitle className="text-destructive">Delete Workspace</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium">Delete this workspace</p>
              <p className="text-xs text-muted-foreground">This action cannot be undone.</p>
            </div>
            <Button
              variant="destructive"
              onClick={() => setDeleteDialogOpen(true)}
              className="cursor-pointer"
              data-testid="workspace-settings-delete-button"
            >
              <IconTrash className="h-4 w-4 mr-2" />
              Delete
            </Button>
          </div>
        </CardContent>
      </Card>

      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Workspace</DialogTitle>
            <DialogDescription>
              Type the workspace name <span className="font-medium">{workspaceName}</span> to
              confirm deletion. This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="confirm-delete">Confirm Delete</Label>
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
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={onDelete}
              disabled={deleteConfirmText !== workspaceName}
              className="cursor-pointer"
              data-testid="workspace-settings-delete-confirm-button"
            >
              Delete
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
        title: "Failed to save workspace",
        description: error instanceof Error ? error.message : "Request failed",
        variant: "error",
      });
      throw error;
    }
  };
}

function useWorkspaceEditForm(workspace: Workspace) {
  const router = useRouter();
  const { toast } = useToast();
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
  });

  const handleDeleteWorkspace = async () => {
    if (deleteConfirmText !== currentWorkspace.name) return;
    try {
      await deleteWorkspaceRequest.run(currentWorkspace.id, currentWorkspace.name);
      setWorkspaces(workspaces.filter((ws: Workspace) => ws.id !== currentWorkspace.id));
      runWithNavigationBlockerBypassed(() => router.push("/settings/workspace"));
    } catch (error) {
      toast({
        title: "Failed to delete workspace",
        description: error instanceof Error ? error.message : "Request failed",
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

  useSettingsSaveContributor({
    id: `workspace:${currentWorkspace.id}`,
    revision: JSON.stringify({
      workspaceNameDraft,
      defaultExecutorId,
      defaultAgentProfileId,
    }),
    isDirty,
    canSave: Boolean(workspaceNameDraft.trim()),
    invalidReason: workspaceNameDraft.trim() ? undefined : "Workspace name is required.",
    save: handleSave,
    discard: handleDiscard,
  });

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">{currentWorkspace.name}</h2>
        <p className="text-sm text-muted-foreground mt-1">
          Manage workspace details and jump into workflows or repositories.
        </p>
      </div>
      <Separator />
      <WorkspaceSettingsCard
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
      <WorkspaceLinksCard workspaceId={currentWorkspace.id} />
      <Separator />
      <DeleteWorkspaceCard
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

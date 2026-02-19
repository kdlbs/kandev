"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
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
import type { Workspace, Executor, Environment } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import { useRequest } from "@/lib/http/use-request";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import { UnsavedChangesBadge, UnsavedSaveButton } from "@/components/settings/unsaved-indicator";

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
  onChange: (v: string) => void;
  options: { id: string; name: string }[];
  emptyLabel: string;
  emptyValue: string;
};

function SelectField({
  label,
  value,
  onChange,
  options,
  emptyLabel,
  emptyValue,
}: SelectFieldProps) {
  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      <Select value={value || "none"} onValueChange={(v) => onChange(v === "none" ? "" : v)}>
        <SelectTrigger className="w-full">
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
  isDirty: boolean;
  workspaceNameDraft: string;
  onNameChange: (value: string) => void;
  defaultExecutorId: string;
  onExecutorChange: (value: string) => void;
  activeExecutors: Executor[];
  executorsEmpty: boolean;
  defaultEnvironmentId: string;
  onEnvironmentChange: (value: string) => void;
  environments: Environment[];
  defaultAgentProfileId: string;
  onAgentProfileChange: (value: string) => void;
  agentProfiles: AgentProfileOption[];
  isLoading: boolean;
  saveStatus: "idle" | "loading" | "success" | "error";
  onSave: () => void;
};

function WorkspaceSettingsCard({
  isDirty,
  workspaceNameDraft,
  onNameChange,
  defaultExecutorId,
  onExecutorChange,
  activeExecutors,
  executorsEmpty,
  defaultEnvironmentId,
  onEnvironmentChange,
  environments,
  defaultAgentProfileId,
  onAgentProfileChange,
  agentProfiles,
  isLoading,
  saveStatus,
  onSave,
}: WorkspaceSettingsCardProps) {
  const executorOptions = activeExecutors.map((e: Executor) => ({ id: e.id, name: e.name }));
  const envOptions = environments.map((e: Environment) => ({ id: e.id, name: e.name }));
  const profileOptions = agentProfiles.map((p: AgentProfileOption) => ({
    id: p.id,
    name: p.label,
  }));
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <span>Workspace Settings</span>
          {isDirty && <UnsavedChangesBadge />}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="workspace-name">Name</Label>
            <Input
              id="workspace-name"
              value={workspaceNameDraft}
              onChange={(e) => onNameChange(e.target.value)}
            />
          </div>
          <SelectField
            label="Default Executor"
            value={defaultExecutorId}
            onChange={onExecutorChange}
            options={executorsEmpty ? [] : executorOptions}
            emptyLabel="No executors available"
            emptyValue=""
          />
          <SelectField
            label="Default Environment"
            value={defaultEnvironmentId}
            onChange={onEnvironmentChange}
            options={envOptions}
            emptyLabel="No environments available"
            emptyValue="empty-environments"
          />
          <SelectField
            label="Default Agent Profile"
            value={defaultAgentProfileId}
            onChange={onAgentProfileChange}
            options={profileOptions}
            emptyLabel="No agent profiles available"
            emptyValue="empty-agent-profiles"
          />
          <div className="flex justify-end pt-2">
            <UnsavedSaveButton
              isDirty={isDirty}
              isLoading={isLoading}
              status={saveStatus}
              onClick={onSave}
            />
          </div>
        </div>
      </CardContent>
    </Card>
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
  deleteDialogOpen: boolean;
  setDeleteDialogOpen: (open: boolean) => void;
  deleteConfirmText: string;
  setDeleteConfirmText: (text: string) => void;
  onDelete: () => void;
};

function DeleteWorkspaceCard({
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
              Type &quot;delete&quot; to confirm deletion. This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="confirm-delete">Confirm Delete</Label>
            <Input
              id="confirm-delete"
              value={deleteConfirmText}
              onChange={(event) => setDeleteConfirmText(event.target.value)}
              placeholder="delete"
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
              disabled={deleteConfirmText !== "delete"}
              className="cursor-pointer"
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
  environmentId: string;
  agentProfileId: string;
};

function buildWorkspaceUpdates(
  draft: { name: string; executorId: string; environmentId: string; agentProfileId: string },
  saved: SavedState,
): Record<string, string | undefined> {
  const updates: Record<string, string | undefined> = {};
  if (draft.name.trim() !== saved.name) updates.name = draft.name.trim();
  if (draft.executorId !== saved.executorId) updates.default_executor_id = draft.executorId;
  if (draft.environmentId !== saved.environmentId)
    updates.default_environment_id = draft.environmentId;
  if (draft.agentProfileId !== saved.agentProfileId)
    updates.default_agent_profile_id = draft.agentProfileId;
  return updates;
}

type WorkspaceDraftState = {
  workspaceNameDraft: string;
  defaultExecutorId: string;
  defaultEnvironmentId: string;
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
          environmentId: draft.defaultEnvironmentId,
          agentProfileId: draft.defaultAgentProfileId,
        },
        savedState,
      );
      const updated = await saveWorkspaceRequest.run(currentWorkspace.id, updates);
      setCurrentWorkspace((prev) => ({ ...prev, ...updated }));
      setSavedState({
        name: updated.name ?? draft.workspaceNameDraft.trim(),
        executorId: updated.default_executor_id ?? "",
        environmentId: updated.default_environment_id ?? "",
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
    }
  };
}

function useWorkspaceEditForm(workspace: Workspace) {
  const router = useRouter();
  const { toast } = useToast();
  const [currentWorkspace, setCurrentWorkspace] = useState<Workspace>(workspace);
  const [workspaceNameDraft, setWorkspaceNameDraft] = useState(workspace.name ?? "");
  const [defaultExecutorId, setDefaultExecutorId] = useState(workspace.default_executor_id ?? "");
  const [defaultEnvironmentId, setDefaultEnvironmentId] = useState(
    workspace.default_environment_id ?? "",
  );
  const [defaultAgentProfileId, setDefaultAgentProfileId] = useState(
    workspace.default_agent_profile_id ?? "",
  );
  const [savedState, setSavedState] = useState<SavedState>({
    name: workspace.name ?? "",
    executorId: workspace.default_executor_id ?? "",
    environmentId: workspace.default_environment_id ?? "",
    agentProfileId: workspace.default_agent_profile_id ?? "",
  });
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState("");

  const executors = useAppStore((state) => state.executors.items);
  const environments = useAppStore((state) => state.environments.items);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const workspaces = useAppStore((state) => state.workspaces.items);
  const setWorkspaces = useAppStore((state) => state.setWorkspaces);

  const saveWorkspaceRequest = useRequest(updateWorkspaceAction);
  const deleteWorkspaceRequest = useRequest(deleteWorkspaceAction);

  const activeExecutors = executors.filter((executor: Executor) => executor.status === "active");
  const isDirty =
    workspaceNameDraft.trim() !== savedState.name ||
    defaultExecutorId !== savedState.executorId ||
    defaultEnvironmentId !== savedState.environmentId ||
    defaultAgentProfileId !== savedState.agentProfileId;

  const handleSave = buildSaveHandler({
    currentWorkspace,
    draft: { workspaceNameDraft, defaultExecutorId, defaultEnvironmentId, defaultAgentProfileId },
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
    if (deleteConfirmText !== "delete") return;
    try {
      await deleteWorkspaceRequest.run(currentWorkspace.id);
      setWorkspaces(workspaces.filter((ws: Workspace) => ws.id !== currentWorkspace.id));
      router.push("/settings/workspace");
    } catch (error) {
      toast({
        title: "Failed to delete workspace",
        description: error instanceof Error ? error.message : "Request failed",
        variant: "error",
      });
    }
  };

  return {
    currentWorkspace,
    workspaceNameDraft,
    setWorkspaceNameDraft,
    defaultExecutorId,
    setDefaultExecutorId,
    defaultEnvironmentId,
    setDefaultEnvironmentId,
    defaultAgentProfileId,
    setDefaultAgentProfileId,
    deleteDialogOpen,
    setDeleteDialogOpen,
    deleteConfirmText,
    setDeleteConfirmText,
    activeExecutors,
    executors,
    environments,
    agentProfiles,
    isDirty,
    saveWorkspaceRequest,
    handleSave,
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
    defaultEnvironmentId,
    setDefaultEnvironmentId,
    defaultAgentProfileId,
    setDefaultAgentProfileId,
    deleteDialogOpen,
    setDeleteDialogOpen,
    deleteConfirmText,
    setDeleteConfirmText,
    activeExecutors,
    executors,
    environments,
    agentProfiles,
    isDirty,
    saveWorkspaceRequest,
    handleSave,
    handleDeleteWorkspace,
  } = useWorkspaceEditForm(workspace);

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
        isDirty={isDirty}
        workspaceNameDraft={workspaceNameDraft}
        onNameChange={setWorkspaceNameDraft}
        defaultExecutorId={defaultExecutorId}
        onExecutorChange={setDefaultExecutorId}
        activeExecutors={activeExecutors}
        executorsEmpty={executors.length === 0}
        defaultEnvironmentId={defaultEnvironmentId}
        onEnvironmentChange={setDefaultEnvironmentId}
        environments={environments}
        defaultAgentProfileId={defaultAgentProfileId}
        onAgentProfileChange={setDefaultAgentProfileId}
        agentProfiles={agentProfiles}
        isLoading={saveWorkspaceRequest.isLoading}
        saveStatus={saveWorkspaceRequest.status}
        onSave={handleSave}
      />
      <WorkspaceLinksCard workspaceId={currentWorkspace.id} />
      <Separator />
      <DeleteWorkspaceCard
        deleteDialogOpen={deleteDialogOpen}
        setDeleteDialogOpen={setDeleteDialogOpen}
        deleteConfirmText={deleteConfirmText}
        setDeleteConfirmText={setDeleteConfirmText}
        onDelete={handleDeleteWorkspace}
      />
    </div>
  );
}

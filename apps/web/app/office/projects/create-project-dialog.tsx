"use client";

import { useState, useCallback } from "react";
import { IconPlus, IconX } from "@tabler/icons-react";
import { toast } from "sonner";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAppStore } from "@/components/state-provider";
import { createProject } from "@/lib/api/domains/office-api";
import type { AgentProfile } from "@/lib/state/slices/office/types";

const COLOR_OPTIONS = [
  "#ef4444",
  "#f97316",
  "#eab308",
  "#22c55e",
  "#06b6d4",
  "#3b82f6",
  "#8b5cf6",
  "#ec4899",
];

type CreateProjectDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string;
};

function ColorPicker({ color, onChange }: { color: string; onChange: (c: string) => void }) {
  return (
    <div className="space-y-2">
      <Label>Color</Label>
      <div className="flex gap-2">
        {COLOR_OPTIONS.map((c) => (
          <button
            key={c}
            type="button"
            className={`h-6 w-6 rounded-sm cursor-pointer transition-all ${
              color === c ? "ring-2 ring-offset-2 ring-primary" : ""
            }`}
            style={{ backgroundColor: c }}
            onClick={() => onChange(c)}
          />
        ))}
      </div>
    </div>
  );
}

function ReposField({
  repos,
  repoInput,
  onRepoInputChange,
  onAddRepo,
  onRemoveRepo,
}: {
  repos: string[];
  repoInput: string;
  onRepoInputChange: (v: string) => void;
  onAddRepo: () => void;
  onRemoveRepo: (r: string) => void;
}) {
  return (
    <div className="space-y-2">
      <Label>Repositories</Label>
      <p className="text-xs text-muted-foreground">
        Git URLs or local paths where agents will work
      </p>
      <div className="flex gap-2">
        <Input
          placeholder="URL or path"
          value={repoInput}
          onChange={(e) => onRepoInputChange(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && (e.preventDefault(), onAddRepo())}
          className="flex-1"
        />
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="outline"
              size="icon"
              onClick={onAddRepo}
              className="cursor-pointer shrink-0"
            >
              <IconPlus className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Add repository</TooltipContent>
        </Tooltip>
      </div>
      {repos.length > 0 && (
        <div className="flex flex-wrap gap-1.5 mt-1">
          {repos.map((repo) => (
            <span
              key={repo}
              className="inline-flex items-center gap-1 rounded-md bg-muted px-2 py-1 text-xs"
            >
              {repo}
              <button
                type="button"
                onClick={() => onRemoveRepo(repo)}
                className="cursor-pointer hover:text-destructive"
              >
                <IconX className="h-3 w-3" />
              </button>
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

const FALLBACK_EXECUTOR_TYPES = [
  { id: "local_pc", label: "Local (standalone)" },
  { id: "local_docker", label: "Local Docker" },
  { id: "sprites", label: "Sprites (remote sandbox)" },
  { id: "remote_docker", label: "Remote Docker" },
];

function ExecutorField({
  executorType,
  dockerImage,
  executorTypes,
  onExecutorTypeChange,
  onDockerImageChange,
}: {
  executorType: string;
  dockerImage: string;
  executorTypes: Array<{ id: string; label: string }>;
  onExecutorTypeChange: (v: string) => void;
  onDockerImageChange: (v: string) => void;
}) {
  return (
    <div className="space-y-2">
      <Label>Executor Type</Label>
      <p className="text-xs text-muted-foreground">
        How agent sessions run (inherit uses workspace default)
      </p>
      <Select value={executorType} onValueChange={onExecutorTypeChange}>
        <SelectTrigger className="cursor-pointer">
          <SelectValue placeholder="Inherit from workspace" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="inherit" className="cursor-pointer">
            Inherit from workspace
          </SelectItem>
          {executorTypes.map((et) => (
            <SelectItem key={et.id} value={et.id} className="cursor-pointer">
              {et.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {(executorType === "local_docker" || executorType === "remote_docker") && (
        <Input
          placeholder="Docker image (e.g. node:20-slim)"
          value={dockerImage}
          onChange={(e) => onDockerImageChange(e.target.value)}
          className="mt-2"
        />
      )}
    </div>
  );
}

type ProjectFormState = {
  name: string;
  description: string;
  color: string;
  repos: string[];
  repoInput: string;
  leadAgentId: string;
  executorType: string;
  dockerImage: string;
};

const INITIAL_PROJECT_STATE: ProjectFormState = {
  name: "",
  description: "",
  color: COLOR_OPTIONS[5],
  repos: [],
  repoInput: "",
  leadAgentId: "",
  executorType: "",
  dockerImage: "",
};

function useProjectForm(workspaceId: string, onClose: () => void) {
  const addProject = useAppStore((s) => s.addProject);
  const [form, setForm] = useState<ProjectFormState>(INITIAL_PROJECT_STATE);
  const [submitting, setSubmitting] = useState(false);

  const update = useCallback(
    (patch: Partial<ProjectFormState>) => setForm((prev) => ({ ...prev, ...patch })),
    [],
  );

  const handleAddRepo = useCallback(() => {
    const trimmed = form.repoInput.trim();
    if (trimmed && !form.repos.includes(trimmed)) {
      update({ repos: [...form.repos, trimmed], repoInput: "" });
    }
  }, [form.repoInput, form.repos, update]);

  const handleRemoveRepo = useCallback(
    (repo: string) => update({ repos: form.repos.filter((r) => r !== repo) }),
    [form.repos, update],
  );

  const handleCreate = useCallback(async () => {
    if (!form.name.trim()) return;
    setSubmitting(true);
    try {
      const result = await createProject(workspaceId, {
        name: form.name.trim(),
        description: form.description,
        color: form.color,
        repositories: form.repos,
        leadAgentProfileId: form.leadAgentId || undefined,
        executorConfig: form.executorType
          ? { type: form.executorType, image: form.dockerImage || undefined }
          : undefined,
      });
      if (result) addProject(result);
      onClose();
      setForm(INITIAL_PROJECT_STATE);
      toast.success("Project created");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create project");
    } finally {
      setSubmitting(false);
    }
  }, [form, workspaceId, addProject, onClose]);

  return { form, update, submitting, handleAddRepo, handleRemoveRepo, handleCreate };
}

function ProjectFormBody({
  form,
  agents,
  executorTypes,
  onUpdate,
  onAddRepo,
  onRemoveRepo,
}: {
  form: ProjectFormState;
  agents: AgentProfile[];
  executorTypes: Array<{ id: string; label: string }>;
  onUpdate: (patch: Partial<ProjectFormState>) => void;
  onAddRepo: () => void;
  onRemoveRepo: (r: string) => void;
}) {
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="project-name">Name</Label>
        <Input
          id="project-name"
          placeholder="Project name"
          value={form.name}
          onChange={(e) => onUpdate({ name: e.target.value })}
          autoFocus
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="project-desc">Description</Label>
        <Textarea
          id="project-desc"
          placeholder="Project description..."
          value={form.description}
          onChange={(e) => onUpdate({ description: e.target.value })}
          className="min-h-[80px]"
        />
      </div>
      <ColorPicker color={form.color} onChange={(c) => onUpdate({ color: c })} />
      <ReposField
        repos={form.repos}
        repoInput={form.repoInput}
        onRepoInputChange={(v) => onUpdate({ repoInput: v })}
        onAddRepo={onAddRepo}
        onRemoveRepo={onRemoveRepo}
      />
      <div className="space-y-2">
        <Label>Lead Agent</Label>
        <p className="text-xs text-muted-foreground">
          The agent responsible for managing this project
        </p>
        <Select value={form.leadAgentId} onValueChange={(v) => onUpdate({ leadAgentId: v })}>
          <SelectTrigger className="cursor-pointer">
            <SelectValue placeholder="Select agent (optional)" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="none" className="cursor-pointer">
              None
            </SelectItem>
            {agents.map((a) => (
              <SelectItem key={a.id} value={a.id} className="cursor-pointer">
                {a.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <ExecutorField
        executorType={form.executorType}
        dockerImage={form.dockerImage}
        executorTypes={executorTypes}
        onExecutorTypeChange={(v) => onUpdate({ executorType: v })}
        onDockerImageChange={(v) => onUpdate({ dockerImage: v })}
      />
    </div>
  );
}

export function CreateProjectDialog({ open, onOpenChange, workspaceId }: CreateProjectDialogProps) {
  const agents = useAppStore((s) => s.office.agentProfiles);
  const meta = useAppStore((s) => s.office.meta);
  const executorTypes =
    meta?.executorTypes.map((e) => ({ id: e.id, label: e.label })) ?? FALLBACK_EXECUTOR_TYPES;
  const { form, update, submitting, handleAddRepo, handleRemoveRepo, handleCreate } =
    useProjectForm(workspaceId, () => onOpenChange(false));

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>New Project</DialogTitle>
        </DialogHeader>

        <ProjectFormBody
          form={form}
          agents={agents}
          executorTypes={executorTypes}
          onUpdate={update}
          onAddRepo={handleAddRepo}
          onRemoveRepo={handleRemoveRepo}
        />
        <div className="flex justify-end gap-2 pt-4 border-t border-border">
          <Button variant="ghost" onClick={() => onOpenChange(false)} className="cursor-pointer">
            Cancel
          </Button>
          <Button
            onClick={handleCreate}
            disabled={!form.name.trim() || submitting}
            className="cursor-pointer"
          >
            {submitting ? "Creating..." : "Create Project"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

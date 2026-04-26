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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@kandev/ui/select";
import { useAppStore } from "@/components/state-provider";
import { createProject } from "@/lib/api/domains/orchestrate-api";

const COLOR_OPTIONS = [
  "#ef4444", "#f97316", "#eab308", "#22c55e",
  "#06b6d4", "#3b82f6", "#8b5cf6", "#ec4899",
];

type CreateProjectDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string;
};

export function CreateProjectDialog({
  open,
  onOpenChange,
  workspaceId,
}: CreateProjectDialogProps) {
  const addProject = useAppStore((s) => s.addProject);
  const agents = useAppStore((s) => s.orchestrate.agentInstances);

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [color, setColor] = useState(COLOR_OPTIONS[5]);
  const [repos, setRepos] = useState<string[]>([]);
  const [repoInput, setRepoInput] = useState("");
  const [leadAgentId, setLeadAgentId] = useState("");
  const [executorType, setExecutorType] = useState("");
  const [dockerImage, setDockerImage] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const handleAddRepo = useCallback(() => {
    const trimmed = repoInput.trim();
    if (trimmed && !repos.includes(trimmed)) {
      setRepos((prev) => [...prev, trimmed]);
      setRepoInput("");
    }
  }, [repoInput, repos]);

  const handleRemoveRepo = useCallback((repo: string) => {
    setRepos((prev) => prev.filter((r) => r !== repo));
  }, []);

  const handleCreate = useCallback(async () => {
    if (!name.trim()) return;
    setSubmitting(true);
    try {
      const result = await createProject(workspaceId, {
        name: name.trim(),
        description,
        color,
        repositories: repos,
        leadAgentInstanceId: leadAgentId || undefined,
        executorConfig: executorType
          ? { type: executorType, image: dockerImage || undefined }
          : undefined,
      });
      if (result) {
        addProject(result);
      }
      onOpenChange(false);
      setName("");
      setDescription("");
      setRepos([]);
      setLeadAgentId("");
      setExecutorType("");
      setDockerImage("");
      toast.success("Project created");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to create project");
    } finally {
      setSubmitting(false);
    }
  }, [
    name, description, color, repos, leadAgentId,
    executorType, dockerImage, workspaceId, addProject, onOpenChange,
  ]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>New Project</DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="project-name">Name</Label>
            <Input
              id="project-name"
              placeholder="Project name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              autoFocus
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="project-desc">Description</Label>
            <Textarea
              id="project-desc"
              placeholder="Project description..."
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              className="min-h-[80px]"
            />
          </div>

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
                  onClick={() => setColor(c)}
                />
              ))}
            </div>
          </div>

          <div className="space-y-2">
            <Label>Repositories</Label>
            <div className="flex gap-2">
              <Input
                placeholder="URL or path"
                value={repoInput}
                onChange={(e) => setRepoInput(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && (e.preventDefault(), handleAddRepo())}
                className="flex-1"
              />
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    onClick={handleAddRepo}
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
                      onClick={() => handleRemoveRepo(repo)}
                      className="cursor-pointer hover:text-destructive"
                    >
                      <IconX className="h-3 w-3" />
                    </button>
                  </span>
                ))}
              </div>
            )}
          </div>

          <div className="space-y-2">
            <Label>Lead Agent</Label>
            <Select value={leadAgentId} onValueChange={setLeadAgentId}>
              <SelectTrigger className="cursor-pointer">
                <SelectValue placeholder="Select agent (optional)" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="none" className="cursor-pointer">None</SelectItem>
                {agents.map((a) => (
                  <SelectItem key={a.id} value={a.id} className="cursor-pointer">
                    {a.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2">
            <Label>Executor Type</Label>
            <Select value={executorType} onValueChange={setExecutorType}>
              <SelectTrigger className="cursor-pointer">
                <SelectValue placeholder="Inherit from workspace" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="inherit" className="cursor-pointer">Inherit from workspace</SelectItem>
                <SelectItem value="local_pc" className="cursor-pointer">Local (standalone)</SelectItem>
                <SelectItem value="local_docker" className="cursor-pointer">Local Docker</SelectItem>
                <SelectItem value="sprites" className="cursor-pointer">Sprites (remote sandbox)</SelectItem>
                <SelectItem value="remote_docker" className="cursor-pointer">Remote Docker</SelectItem>
              </SelectContent>
            </Select>
            {(executorType === "local_docker" || executorType === "remote_docker") && (
              <Input
                placeholder="Docker image (e.g. node:20-slim)"
                value={dockerImage}
                onChange={(e) => setDockerImage(e.target.value)}
                className="mt-2"
              />
            )}
          </div>
        </div>

        <div className="flex justify-end gap-2 pt-4 border-t border-border">
          <Button
            variant="ghost"
            onClick={() => onOpenChange(false)}
            className="cursor-pointer"
          >
            Cancel
          </Button>
          <Button
            onClick={handleCreate}
            disabled={!name.trim() || submitting}
            className="cursor-pointer"
          >
            {submitting ? "Creating..." : "Create Project"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

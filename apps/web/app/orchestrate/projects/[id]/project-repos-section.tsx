"use client";

import { useState, useCallback } from "react";
import { IconPlus, IconX, IconGitBranch } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { toast } from "sonner";
import { updateProject } from "@/lib/api/domains/orchestrate-api";
import { useAppStore } from "@/components/state-provider";
import type { Project } from "@/lib/state/slices/orchestrate/types";

type ProjectReposSectionProps = {
  project: Project;
};

export function ProjectReposSection({ project }: ProjectReposSectionProps) {
  const updateProjectStore = useAppStore((s) => s.updateProject);
  const [repoInput, setRepoInput] = useState("");
  const repos = project.repositories ?? [];

  const handleAdd = useCallback(async () => {
    const trimmed = repoInput.trim();
    if (!trimmed || repos.includes(trimmed)) return;
    const updated = [...repos, trimmed];
    try {
      await updateProject(project.id, { repositories: updated });
      updateProjectStore(project.id, { repositories: updated });
      setRepoInput("");
      toast.success("Repository added");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to add repository");
    }
  }, [repoInput, repos, project.id, updateProjectStore]);

  const handleRemove = useCallback(
    async (repo: string) => {
      const updated = repos.filter((r) => r !== repo);
      try {
        await updateProject(project.id, { repositories: updated });
        updateProjectStore(project.id, { repositories: updated });
        toast.success("Repository removed");
      } catch (err) {
        toast.error(err instanceof Error ? err.message : "Failed to remove repository");
      }
    },
    [repos, project.id, updateProjectStore],
  );

  return (
    <div className="space-y-3">
      <h2 className="text-sm font-semibold">Repositories</h2>
      <p className="text-xs text-muted-foreground">
        Git URLs or local paths where agents will work on this project.
      </p>
      <div className="flex gap-2">
        <Input
          placeholder="Repository URL or path"
          value={repoInput}
          onChange={(e) => setRepoInput(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && (e.preventDefault(), handleAdd())}
          className="flex-1"
        />
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="outline"
              size="icon"
              onClick={handleAdd}
              className="cursor-pointer shrink-0"
            >
              <IconPlus className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Add repository</TooltipContent>
        </Tooltip>
      </div>
      {repos.length === 0 ? (
        <p className="text-xs text-muted-foreground">No repositories added yet</p>
      ) : (
        <ul className="space-y-1">
          {repos.map((repo) => (
            <li
              key={repo}
              className="flex items-center gap-2 text-sm py-1.5 px-2 rounded-md bg-muted/50"
            >
              <IconGitBranch className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
              <span className="flex-1 truncate font-mono text-xs">{repo}</span>
              <button
                type="button"
                onClick={() => handleRemove(repo)}
                className="cursor-pointer text-muted-foreground hover:text-destructive shrink-0"
              >
                <IconX className="h-3.5 w-3.5" />
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

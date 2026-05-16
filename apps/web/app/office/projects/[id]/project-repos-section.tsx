"use client";

import { useCallback, useMemo } from "react";
import { IconCode, IconWorld, IconX } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { toast } from "sonner";
import { updateProject } from "@/lib/api/domains/office-api";
import { useAppStore } from "@/components/state-provider";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { formatUserHomePath } from "@/lib/utils";
import type { Project } from "@/lib/state/slices/office/types";
import type { Repository } from "@/lib/types/http";
import { ProjectRepositoryPicker } from "./project-repository-picker";
import { normalizeRepos } from "../normalize-repos";

type ProjectReposSectionProps = {
  project: Project;
};

export function ProjectReposSection({ project }: ProjectReposSectionProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const updateProjectStore = useAppStore((s) => s.updateProject);
  const { repositories } = useRepositories(workspaceId);
  const repos = useMemo(() => normalizeRepos(project.repositories), [project.repositories]);

  const persist = useCallback(
    async (next: string[], successMessage: string, failureMessage: string) => {
      try {
        await updateProject(project.id, { repositories: next });
        updateProjectStore(project.id, { repositories: next });
        toast.success(successMessage);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : failureMessage);
      }
    },
    [project.id, updateProjectStore],
  );

  const handleAdd = useCallback(
    (value: string) => {
      const trimmed = value.trim();
      if (!trimmed || repos.includes(trimmed)) return;
      void persist([...repos, trimmed], "Repository added", "Failed to add repository");
    },
    [repos, persist],
  );

  const handleRemove = useCallback(
    (repo: string) => {
      void persist(
        repos.filter((r) => r !== repo),
        "Repository removed",
        "Failed to remove repository",
      );
    },
    [repos, persist],
  );

  return (
    <div className="space-y-3">
      <div>
        <h2 className="text-sm font-semibold">Repositories</h2>
        <p className="text-xs text-muted-foreground">
          Git URLs or local paths where agents will work on this project.
        </p>
      </div>
      <div className="flex flex-wrap items-center gap-2" data-testid="project-repo-chips">
        {repos.map((repo) => (
          <RepoChip
            key={repo}
            value={repo}
            workspaceRepos={repositories}
            onRemove={() => handleRemove(repo)}
          />
        ))}
        <ProjectRepositoryPicker workspaceId={workspaceId} exclude={repos} onSelect={handleAdd} />
      </div>
      {repos.length === 0 && (
        <p className="text-xs text-muted-foreground">No repositories added yet.</p>
      )}
    </div>
  );
}

/**
 * A single attached repository, rendered as a chip with friendly label
 * + remove button. Falls back to the raw stored string when no
 * workspace row matches (custom URL or unimported local path).
 */
function RepoChip({
  value,
  workspaceRepos,
  onRemove,
}: {
  value: string;
  workspaceRepos: Repository[];
  onRemove: () => void;
}) {
  const matched = workspaceRepos.find((r) => r.local_path === value);
  const isUrl = looksLikeUrl(value);
  const label = matched?.name ?? (isUrl ? value : leafSegment(value));
  const detail = matched?.local_path ?? value;
  const displayDetail = isUrl ? value : formatUserHomePath(detail);

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className="inline-flex items-center gap-1.5 rounded-md border border-input bg-input/20 dark:bg-input/30 pl-2.5 pr-0.5 h-8 text-xs"
          data-testid="project-repo-chip"
          data-repository-value={value}
        >
          {isUrl ? (
            <IconWorld className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          ) : (
            <IconCode className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          )}
          <span className="truncate max-w-[240px]">{label}</span>
          <button
            type="button"
            onClick={onRemove}
            aria-label="Remove repository"
            className="h-6 w-6 inline-flex items-center justify-center rounded text-muted-foreground hover:text-destructive hover:bg-muted/60 cursor-pointer"
            data-testid="project-repo-chip-remove"
          >
            <IconX className="h-3 w-3" />
          </button>
        </span>
      </TooltipTrigger>
      <TooltipContent>{displayDetail}</TooltipContent>
    </Tooltip>
  );
}

function looksLikeUrl(value: string): boolean {
  return /^(https?:\/\/|git@|ssh:\/\/|git:\/\/)/i.test(value);
}

function leafSegment(path: string): string {
  const cleaned = path.replace(/\\/g, "/").replace(/\/+$/g, "");
  const idx = cleaned.lastIndexOf("/");
  return idx >= 0 ? cleaned.slice(idx + 1) : cleaned;
}

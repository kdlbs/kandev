"use client";

import { useCallback, useMemo } from "react";
import { toast } from "sonner";
import { updateProject } from "@/lib/api/domains/office-api";
import { useAppStore } from "@/components/state-provider";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import type { Project } from "@/lib/state/slices/office/types";
import { ProjectRepositoryPicker } from "../project-repository-picker";
import { RepoChip } from "../repo-chip";
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
        <ProjectRepositoryPicker
          workspaceId={workspaceId}
          repositories={repositories}
          exclude={repos}
          onSelect={handleAdd}
        />
      </div>
      {repos.length === 0 && (
        <p className="text-xs text-muted-foreground">No repositories added yet.</p>
      )}
    </div>
  );
}

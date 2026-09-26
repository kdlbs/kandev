"use client";

import { useCallback, useMemo } from "react";
import { toast } from "@/lib/toast/sonner";
import { updateProject } from "@/lib/api/domains/office-api";
import { useAppStore } from "@/components/state-provider";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import type { Project } from "@/lib/state/slices/office/types";
import { ProjectRepositoryPicker } from "../project-repository-picker";
import { RepoChip } from "../repo-chip";
import { normalizeRepos } from "../normalize-repos";
import { useTranslation } from "react-i18next";

type ProjectReposSectionProps = {
  project: Project;
};

export function ProjectReposSection({ project }: ProjectReposSectionProps) {
  const { t } = useTranslation();
  const updateProjectStore = useAppStore((s) => s.updateProject);
  const { repositories } = useRepositories(project.workspaceId);
  const repos = useMemo(() => normalizeRepos(project.repositories), [project.repositories]);

  // Catalog KEYS, not messages: the call sites are memoized callbacks, so a
  // pre-resolved English string would be captured once and never re-resolve.
  const persist = useCallback(
    async (next: string[], successKey: string, failureKey: string) => {
      try {
        await updateProject(project.id, { repositories: next });
        updateProjectStore(project.workspaceId, project.id, { repositories: next });
        toast.success(t(successKey));
      } catch (err) {
        toast.error(err instanceof Error ? err.message : t(failureKey));
      }
    },
    [project.id, project.workspaceId, updateProjectStore, t],
  );

  const handleAdd = useCallback(
    (value: string) => {
      const trimmed = value.trim();
      if (!trimmed || repos.includes(trimmed)) return;
      void persist([...repos, trimmed], "office:repositoryAdded", "office:failedToAddRepository");
    },
    [repos, persist],
  );

  const handleRemove = useCallback(
    (repo: string) => {
      void persist(
        repos.filter((r) => r !== repo),
        "office:repositoryRemoved",
        "office:failedToRemoveRepository",
      );
    },
    [repos, persist],
  );

  return (
    <div className="space-y-3">
      <div>
        <h2 className="text-sm font-semibold">{t("office:repositories")}</h2>
        <p className="text-xs text-muted-foreground">{t("office:gitUrlsOrLocalPathsWhere")}</p>
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
          workspaceId={project.workspaceId}
          repositories={repositories}
          exclude={repos}
          onSelect={handleAdd}
        />
      </div>
      {repos.length === 0 && (
        <p className="text-xs text-muted-foreground">{t("office:noRepositoriesAddedYet")}</p>
      )}
    </div>
  );
}

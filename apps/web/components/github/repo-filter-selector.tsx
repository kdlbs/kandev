"use client";

import { useState, useEffect, useCallback, useMemo } from "react";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { fetchGitHubWorkspaceSettings } from "@/lib/api/domains/github-api";
import type { RepoFilter, GitHubWorkspaceSettings } from "@/lib/types/github";
import { orgAllowedByScope } from "@/lib/github/repo-scope";
import { GitHubOrganizationMultiSelect } from "./github-organization-multi-select";
import { GitHubRepositoryMultiSelect } from "./github-repository-multi-select";
import { useTranslation } from "react-i18next";

type RepoFilterSelectorProps = {
  allRepos: boolean;
  selectedRepos: RepoFilter[];
  onAllReposChange: (checked: boolean) => void;
  onSelectedReposChange: (repos: RepoFilter[]) => void;
  workspaceId?: string;
};

function useWorkspaceRepoScope(workspaceId: string | undefined) {
  const [settings, setSettings] = useState<GitHubWorkspaceSettings | null>(null);

  useEffect(() => {
    if (!workspaceId) {
      setSettings(null);
      return;
    }
    let cancelled = false;
    void fetchGitHubWorkspaceSettings(workspaceId)
      .then((next) => {
        if (!cancelled) setSettings(next);
      })
      .catch(() => {
        if (!cancelled) setSettings(null);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId]);

  return settings;
}

const isOrgEntry = (filter: RepoFilter) => filter.name === "";

function useSplitSelection(
  selectedRepos: RepoFilter[],
  onSelectedReposChange: (repos: RepoFilter[]) => void,
) {
  return useMemo(() => {
    const orgEntries = selectedRepos.filter(isOrgEntry);
    const repoEntries = selectedRepos.filter((filter) => !isOrgEntry(filter));
    return {
      orgLogins: orgEntries.map((filter) => filter.owner),
      repoEntries,
      setOrgs: (logins: string[]) =>
        onSelectedReposChange([...logins.map((owner) => ({ owner, name: "" })), ...repoEntries]),
      setRepos: (repos: RepoFilter[]) => onSelectedReposChange([...orgEntries, ...repos]),
    };
  }, [onSelectedReposChange, selectedRepos]);
}

export function RepoFilterSelector({
  allRepos,
  selectedRepos,
  onAllReposChange,
  onSelectedReposChange,
  workspaceId,
}: RepoFilterSelectorProps) {
  const { t } = useTranslation();
  const scope = useWorkspaceRepoScope(workspaceId);
  const showOrgs = !scope || scope.repo_scope_mode !== "repos";
  const split = useSplitSelection(selectedRepos, onSelectedReposChange);
  const isOrgAllowed = useCallback((login: string) => orgAllowedByScope(scope, login), [scope]);

  return (
    <div className="space-y-3">
      <div>
        <Label>{t("github:repositories")}</Label>
        <p className="text-xs text-muted-foreground">
          {t("github:whichAllowedGithubRepositoriesToMonitor")}
        </p>
      </div>

      <div className="flex items-center gap-2">
        <Switch
          id="all-repos-toggle"
          checked={allRepos}
          onCheckedChange={onAllReposChange}
          className="cursor-pointer"
        />
        <Label htmlFor="all-repos-toggle" className="font-normal cursor-pointer">
          {t("github:allRepositoriesAllowedByThisWorkspace")}
        </Label>
      </div>

      {!allRepos && (
        <>
          {showOrgs && (
            <div className="space-y-1.5">
              <Label className="text-xs text-muted-foreground font-normal">
                {t("github:organizations")}
              </Label>
              <GitHubOrganizationMultiSelect
                workspaceId={workspaceId}
                selected={split.orgLogins}
                onChange={split.setOrgs}
                isOrgAllowed={isOrgAllowed}
              />
            </div>
          )}

          <div className="space-y-1.5">
            {showOrgs && (
              <Label className="text-xs text-muted-foreground font-normal">
                {t("github:repositories")}
              </Label>
            )}
            <GitHubRepositoryMultiSelect
              workspaceId={workspaceId}
              selected={showOrgs ? split.repoEntries : selectedRepos}
              onChange={showOrgs ? split.setRepos : onSelectedReposChange}
              scope={scope}
            />
          </div>
        </>
      )}
    </div>
  );
}

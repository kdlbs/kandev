"use client";

import { useState, useEffect, useCallback, useMemo } from "react";
import { IconLoader2 } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { cn } from "@/lib/utils";
import { fetchGitHubWorkspaceSettings, listUserOrgs } from "@/lib/api/domains/github-api";
import type { RepoFilter, GitHubOrg, GitHubWorkspaceSettings } from "@/lib/types/github";
import { orgAllowedByScope } from "@/lib/github/repo-scope";
import { GitHubRepositoryMultiSelect } from "./github-repository-multi-select";
import { useTranslation } from "react-i18next";

type RepoFilterSelectorProps = {
  allRepos: boolean;
  selectedRepos: RepoFilter[];
  onAllReposChange: (checked: boolean) => void;
  onSelectedReposChange: (repos: RepoFilter[]) => void;
  workspaceId?: string;
};

function useGitHubOrgs(workspaceId: string | undefined) {
  const [orgs, setOrgs] = useState<GitHubOrg[]>([]);
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    if (!workspaceId) {
      setOrgs([]);
      setLoading(false);
      return;
    }
    let cancelled = false;
    setLoading(true);
    listUserOrgs(workspaceId)
      .then((r) => {
        if (!cancelled) setOrgs(r.orgs ?? []);
      })
      .catch(() => {
        if (!cancelled) setOrgs([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId]);

  return { orgs, loading };
}

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

function OrgBadges({
  orgs,
  loading,
  selectedRepos,
  disabled,
  onToggleOrg,
}: {
  orgs: GitHubOrg[];
  loading: boolean;
  selectedRepos: RepoFilter[];
  disabled: boolean;
  onToggleOrg: (login: string) => void;
}) {
  const { t } = useTranslation();
  if (loading) {
    return (
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <IconLoader2 className="h-3 w-3 animate-spin" />
        {t("github:loadingOrganizations")}
      </div>
    );
  }
  if (orgs.length === 0) return null;

  return (
    <div className="flex flex-wrap gap-1.5">
      {orgs.map((org) => {
        const isSelected = selectedRepos.some((r) => r.owner === org.login && r.name === "");
        return (
          <Badge
            key={org.login}
            variant={isSelected ? "default" : "outline"}
            className={cn(
              "text-xs cursor-pointer select-none",
              disabled && "opacity-50 pointer-events-none",
            )}
            onClick={() => onToggleOrg(org.login)}
          >
            {org.login}
          </Badge>
        );
      })}
    </div>
  );
}

export function RepoFilterSelector({
  allRepos,
  selectedRepos,
  onAllReposChange,
  onSelectedReposChange,
  workspaceId,
}: RepoFilterSelectorProps) {
  const { t } = useTranslation();
  const { orgs, loading: orgsLoading } = useGitHubOrgs(workspaceId);
  const scope = useWorkspaceRepoScope(workspaceId);
  const scopedOrgs = useMemo(
    () => orgs.filter((org) => orgAllowedByScope(scope, org.login)),
    [orgs, scope],
  );
  const showOrgBadges = !scope || scope.repo_scope_mode !== "repos";

  const toggleOrg = useCallback(
    (login: string) => {
      const exists = selectedRepos.some((r) => r.owner === login && r.name === "");
      if (exists) {
        onSelectedReposChange(selectedRepos.filter((r) => !(r.owner === login && r.name === "")));
      } else {
        onSelectedReposChange([...selectedRepos, { owner: login, name: "" }]);
      }
    },
    [selectedRepos, onSelectedReposChange],
  );

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
          {showOrgBadges && (
            <div className="space-y-1.5">
              <Label className="text-xs text-muted-foreground font-normal">
                {t("github:organizations")}
              </Label>
              <OrgBadges
                orgs={scopedOrgs}
                loading={orgsLoading}
                selectedRepos={selectedRepos}
                disabled={allRepos}
                onToggleOrg={toggleOrg}
              />
            </div>
          )}

          <GitHubRepositoryMultiSelect
            workspaceId={workspaceId}
            selected={selectedRepos}
            onChange={onSelectedReposChange}
            scope={scope}
            disabled={allRepos}
          />
        </>
      )}
    </div>
  );
}

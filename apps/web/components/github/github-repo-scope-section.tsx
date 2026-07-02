"use client";

import { useEffect, useMemo, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import {
  fetchGitHubWorkspaceSettings,
  updateGitHubWorkspaceSettings,
} from "@/lib/api/domains/github-api";
import type { GitHubRepoScopeMode, RepoFilter } from "@/lib/types/github";

function splitCSV(value: string): string[] {
  return value
    .split(",")
    .map((part) => part.trim())
    .filter(Boolean);
}

function parseRepoFilters(value: string): RepoFilter[] {
  return splitCSV(value)
    .map((repo) => {
      const [owner, name, ...rest] = repo.split("/");
      if (!owner || !name || rest.length > 0) return null;
      return { owner, name };
    })
    .filter((repo): repo is RepoFilter => repo !== null);
}

function repoFiltersToInput(repos: RepoFilter[]): string {
  return repos.map((repo) => `${repo.owner}/${repo.name}`).join(", ");
}

type ScopeFieldsProps = {
  mode: GitHubRepoScopeMode;
  orgs: string;
  repos: string;
  loading: boolean;
  invalidRepos: boolean;
  onModeChange: (mode: GitHubRepoScopeMode) => void;
  onOrgsChange: (orgs: string) => void;
  onReposChange: (repos: string) => void;
};

function RepositoryScopeFields({
  mode,
  orgs,
  repos,
  loading,
  invalidRepos,
  onModeChange,
  onOrgsChange,
  onReposChange,
}: ScopeFieldsProps) {
  return (
    <Card>
      <CardContent className="grid gap-4 py-4 md:grid-cols-[220px_minmax(0,1fr)]">
        <div className="space-y-1.5">
          <label className="text-sm font-medium" htmlFor="github-scope-mode">
            Mode
          </label>
          <Select
            value={mode}
            onValueChange={(value) => onModeChange(value as GitHubRepoScopeMode)}
            disabled={loading}
          >
            <SelectTrigger id="github-scope-mode" data-testid="github-scope-mode">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All repositories</SelectItem>
              <SelectItem value="orgs">Organizations</SelectItem>
              <SelectItem value="repos">Selected repositories</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="grid gap-3">
          <div className="space-y-1.5">
            <label className="text-sm font-medium" htmlFor="github-scope-orgs">
              Organizations
            </label>
            <Input
              id="github-scope-orgs"
              value={orgs}
              onChange={(event) => onOrgsChange(event.target.value)}
              disabled={loading || mode !== "orgs"}
              placeholder="kdlbs, example-org"
              data-testid="github-scope-orgs-input"
            />
          </div>
          <div className="space-y-1.5">
            <label className="text-sm font-medium" htmlFor="github-scope-repos">
              Repositories
            </label>
            <Input
              id="github-scope-repos"
              value={repos}
              onChange={(event) => onReposChange(event.target.value)}
              disabled={loading || mode !== "repos"}
              aria-invalid={invalidRepos}
              placeholder="kdlbs/kandev, example/api"
              data-testid="github-scope-repos-input"
            />
            {invalidRepos && (
              <p className="text-xs text-destructive">Use comma-separated owner/repo values.</p>
            )}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

export function GitHubRepoScopeSection({ workspaceId }: { workspaceId: string }) {
  const { toast } = useToast();
  const [mode, setMode] = useState<GitHubRepoScopeMode>("all");
  const [orgs, setOrgs] = useState("");
  const [repos, setRepos] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const parsedRepos = useMemo(() => parseRepoFilters(repos), [repos]);
  const invalidRepos = useMemo(() => {
    const entries = splitCSV(repos);
    return entries.length > 0 && parsedRepos.length !== entries.length;
  }, [parsedRepos.length, repos]);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    void fetchGitHubWorkspaceSettings(workspaceId)
      .then((settings) => {
        if (cancelled) return;
        setMode(settings.repo_scope_mode ?? "all");
        setOrgs((settings.repo_scope_orgs ?? []).join(", "));
        setRepos(repoFiltersToInput(settings.repo_scope_repos ?? []));
      })
      .catch(() => {
        if (!cancelled)
          toast({ description: "Failed to load GitHub workspace settings", variant: "error" });
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [toast, workspaceId]);

  const save = async () => {
    if (invalidRepos) {
      toast({ description: "Repository filters must use owner/repo format", variant: "error" });
      return;
    }
    setSaving(true);
    try {
      const updated = await updateGitHubWorkspaceSettings({
        workspace_id: workspaceId,
        repo_scope_mode: mode,
        repo_scope_orgs: splitCSV(orgs),
        repo_scope_repos: parsedRepos,
      });
      setMode(updated.repo_scope_mode);
      setOrgs((updated.repo_scope_orgs ?? []).join(", "));
      setRepos(repoFiltersToInput(updated.repo_scope_repos ?? []));
      toast({ description: "GitHub workspace settings saved", variant: "success" });
    } catch {
      toast({ description: "Failed to save GitHub workspace settings", variant: "error" });
    } finally {
      setSaving(false);
    }
  };

  return (
    <SettingsSection
      title="Repository Scope"
      description="Choose which GitHub repositories belong to this workspace."
      action={
        <Button
          size="sm"
          onClick={save}
          disabled={loading || saving}
          data-testid="github-scope-save"
          className="cursor-pointer"
        >
          {saving ? "Saving..." : "Save"}
        </Button>
      }
    >
      <RepositoryScopeFields
        mode={mode}
        orgs={orgs}
        repos={repos}
        loading={loading}
        invalidRepos={invalidRepos}
        onModeChange={setMode}
        onOrgsChange={setOrgs}
        onReposChange={setRepos}
      />
    </SettingsSection>
  );
}

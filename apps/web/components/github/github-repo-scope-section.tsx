"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import { SettingsCard } from "@/components/settings/settings-card";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import {
  fetchGitHubWorkspaceSettings,
  updateGitHubWorkspaceSettings,
} from "@/lib/api/domains/github-api";
import type {
  GitHubRepoScopeMode,
  RepoFilter,
  UpdateGitHubWorkspaceSettingsRequest,
} from "@/lib/types/github";

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

const repositoryScopeHelp =
  "Limits the GitHub pull requests and issues Kandev discovers for this workspace, including My GitHub results and review and issue watches. It does not change GitHub permissions or repository access.";

function RepositoryScopeHelp() {
  const usesTouchDrawer = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const button = (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className="h-11 w-11 cursor-pointer text-muted-foreground sm:h-7 sm:w-7"
      aria-haspopup="dialog"
      aria-expanded={open}
      aria-label="Explain repository scope"
    >
      <IconInfoCircle className="h-4 w-4" />
    </Button>
  );
  const drawerTrigger = <DrawerTrigger asChild>{button}</DrawerTrigger>;
  const trigger = usesTouchDrawer ? (
    drawerTrigger
  ) : (
    <Tooltip>
      <TooltipTrigger asChild>{drawerTrigger}</TooltipTrigger>
      <TooltipContent side="top" align="start" className="max-w-[320px] text-xs leading-relaxed">
        {repositoryScopeHelp}
      </TooltipContent>
    </Tooltip>
  );

  return (
    <Drawer open={open} onOpenChange={setOpen}>
      {trigger}
      <DrawerContent>
        <DrawerHeader>
          <DrawerTitle>Repository Scope</DrawerTitle>
          <DrawerDescription>{repositoryScopeHelp}</DrawerDescription>
        </DrawerHeader>
      </DrawerContent>
    </Drawer>
  );
}

type ScopeFieldsProps = {
  mode: GitHubRepoScopeMode;
  orgs: string;
  repos: string;
  baseline: { mode: GitHubRepoScopeMode; orgs: string; repos: string };
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
  baseline,
  loading,
  invalidRepos,
  onModeChange,
  onOrgsChange,
  onReposChange,
}: ScopeFieldsProps) {
  return (
    <SettingsCard
      isDirty={mode !== baseline.mode || orgs !== baseline.orgs || repos !== baseline.repos}
    >
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
            <SelectTrigger
              id="github-scope-mode"
              data-testid="github-scope-mode"
              data-settings-dirty={mode !== baseline.mode}
            >
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
              data-settings-dirty={orgs !== baseline.orgs}
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
              data-settings-dirty={repos !== baseline.repos}
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
    </SettingsCard>
  );
}

function useGitHubRepoScopeDraft(workspaceId: string) {
  const { toast } = useToast();
  const [mode, setMode] = useState<GitHubRepoScopeMode>("all");
  const [orgs, setOrgs] = useState("");
  const [repos, setRepos] = useState("");
  const [baseline, setBaseline] = useState({
    mode: "all" as GitHubRepoScopeMode,
    orgs: "",
    repos: "",
  });
  const [loading, setLoading] = useState(true);
  const parsedRepos = useMemo(() => parseRepoFilters(repos), [repos]);
  const invalidRepos = useMemo(() => {
    const entries = splitCSV(repos);
    return mode === "repos" && entries.length > 0 && parsedRepos.length !== entries.length;
  }, [mode, parsedRepos.length, repos]);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    void fetchGitHubWorkspaceSettings(workspaceId)
      .then((settings) => {
        if (cancelled) return;
        const next = {
          mode: settings.repo_scope_mode ?? "all",
          orgs: (settings.repo_scope_orgs ?? []).join(", "),
          repos: repoFiltersToInput(settings.repo_scope_repos ?? []),
        };
        setBaseline(next);
        setMode(next.mode);
        setOrgs(next.orgs);
        setRepos(next.repos);
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

  const save = useCallback(async () => {
    const submitted = { mode, orgs, repos };
    try {
      const payload: UpdateGitHubWorkspaceSettingsRequest = {
        workspace_id: workspaceId,
        repo_scope_mode: mode,
      };
      if (mode === "orgs") {
        payload.repo_scope_orgs = splitCSV(orgs);
      }
      if (mode === "repos") {
        payload.repo_scope_repos = parsedRepos;
      }
      const updated = await updateGitHubWorkspaceSettings(payload);
      const saved = {
        mode: updated.repo_scope_mode,
        orgs: (updated.repo_scope_orgs ?? []).join(", "),
        repos: repoFiltersToInput(updated.repo_scope_repos ?? []),
      };
      setBaseline(saved);
      setMode((current) => (current === submitted.mode ? saved.mode : current));
      setOrgs((current) => (current === submitted.orgs ? saved.orgs : current));
      setRepos((current) => (current === submitted.repos ? saved.repos : current));
      toast({ description: "GitHub workspace settings saved", variant: "success" });
    } catch {
      toast({ description: "Failed to save GitHub workspace settings", variant: "error" });
      throw new Error("Failed to save GitHub workspace settings");
    }
  }, [mode, orgs, parsedRepos, repos, toast, workspaceId]);
  const discard = useCallback(() => {
    setMode(baseline.mode);
    setOrgs(baseline.orgs);
    setRepos(baseline.repos);
  }, [baseline]);
  const revision = JSON.stringify([mode, orgs, repos]);
  const dirty = revision !== JSON.stringify([baseline.mode, baseline.orgs, baseline.repos]);

  useSettingsSaveContributor({
    id: `github-repo-scope:${workspaceId}`,
    revision,
    isDirty: dirty,
    canSave: !loading && !invalidRepos,
    invalidReason: invalidRepos ? "Use comma-separated owner/repo values." : undefined,
    save,
    discard,
  });

  return {
    mode,
    orgs,
    repos,
    baseline,
    loading,
    invalidRepos,
    setMode,
    setOrgs,
    setRepos,
  };
}

export function GitHubRepoScopeSection({ workspaceId }: { workspaceId: string }) {
  const draft = useGitHubRepoScopeDraft(workspaceId);

  return (
    <SettingsSection
      title="Repository Scope"
      titleAccessory={<RepositoryScopeHelp />}
      description="Limits GitHub pull requests and issues shown or imported in this workspace."
    >
      <RepositoryScopeFields
        mode={draft.mode}
        orgs={draft.orgs}
        repos={draft.repos}
        baseline={draft.baseline}
        loading={draft.loading}
        invalidRepos={draft.invalidRepos}
        onModeChange={draft.setMode}
        onOrgsChange={draft.setOrgs}
        onReposChange={draft.setRepos}
      />
    </SettingsSection>
  );
}

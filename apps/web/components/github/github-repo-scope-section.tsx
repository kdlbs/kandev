"use client";

import { useCallback, useEffect, useState } from "react";
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useToast } from "@/components/toast-provider";
import { SettingsFieldLabel } from "@/components/settings/settings-typography";
import { settingsControlClassName } from "@/components/settings/settings-control";
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
import { useConnectionRequestGuard } from "./github-connection-draft";
import { GitHubOrganizationMultiSelect } from "./github-organization-multi-select";
import { GitHubRepositoryMultiSelect } from "./github-repository-multi-select";
import { useTranslation } from "react-i18next";
import { t as translate } from "@/lib/i18n";
import { controlSizingClassName } from "@kandev/ui/control-sizing";

function orgListKey(orgs: string[]): string {
  return orgs.join(",");
}

function repoListKey(repos: RepoFilter[]): string {
  return repos.map((repo) => `${repo.owner}/${repo.name}`).join(",");
}

type ScopeDraft = { mode: GitHubRepoScopeMode; orgs: string[]; repos: RepoFilter[] };

function draftRevision(draft: ScopeDraft): string {
  return JSON.stringify([draft.mode, orgListKey(draft.orgs), repoListKey(draft.repos)]);
}

// Only the list matching the selected mode is sent; the other stays untouched
// server-side.
function scopePayload({
  workspaceId,
  mode,
  orgs,
  repos,
}: {
  workspaceId: string;
  mode: GitHubRepoScopeMode;
  orgs: string[];
  repos: RepoFilter[];
}): UpdateGitHubWorkspaceSettingsRequest {
  const payload: UpdateGitHubWorkspaceSettingsRequest = {
    workspace_id: workspaceId,
    repo_scope_mode: mode,
  };
  if (mode === "orgs") payload.repo_scope_orgs = orgs;
  if (mode === "repos") payload.repo_scope_repos = repos;
  return payload;
}

function RepositoryScopeHelp() {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const button = (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className={controlSizingClassName("icon", "cursor-pointer text-muted-foreground")}
      aria-haspopup="dialog"
      aria-expanded={open}
      aria-label={t("github:explainRepositoryScope")}
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
        {t("github:repositoryScopeHelp")}
      </TooltipContent>
    </Tooltip>
  );

  return (
    <Drawer open={open} onOpenChange={setOpen}>
      {trigger}
      <DrawerContent>
        <DrawerHeader>
          <DrawerTitle>{t("github:repositoryScope")}</DrawerTitle>
          <DrawerDescription>{t("github:repositoryScopeHelp")}</DrawerDescription>
        </DrawerHeader>
      </DrawerContent>
    </Drawer>
  );
}

type ScopeFieldsProps = {
  workspaceId: string;
  mode: GitHubRepoScopeMode;
  orgs: string[];
  repos: RepoFilter[];
  baseline: ScopeDraft;
  loading: boolean;
  onModeChange: (mode: GitHubRepoScopeMode) => void;
  onOrgsChange: (orgs: string[]) => void;
  onReposChange: (repos: RepoFilter[]) => void;
};

function RepositoryScopeFields({
  workspaceId,
  mode,
  orgs,
  repos,
  baseline,
  loading,
  onModeChange,
  onOrgsChange,
  onReposChange,
}: ScopeFieldsProps) {
  const { t } = useTranslation();
  const orgsDirty = orgListKey(orgs) !== orgListKey(baseline.orgs);
  const reposDirty = repoListKey(repos) !== repoListKey(baseline.repos);
  return (
    <SettingsCard isDirty={mode !== baseline.mode || orgsDirty || reposDirty}>
      <CardContent className="grid gap-4 py-4 md:grid-cols-[220px_minmax(0,1fr)]">
        <div className="space-y-1.5">
          <SettingsFieldLabel htmlFor="github-scope-mode">{t("github:mode")}</SettingsFieldLabel>
          <Select
            value={mode}
            onValueChange={(value) => onModeChange(value as GitHubRepoScopeMode)}
            disabled={loading}
          >
            <SelectTrigger
              id="github-scope-mode"
              data-testid="github-scope-mode"
              data-settings-dirty={mode !== baseline.mode}
              className={settingsControlClassName()}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("github:allRepositories")}</SelectItem>
              <SelectItem value="orgs">{t("github:organizations")}</SelectItem>
              <SelectItem value="repos">{t("github:selectedRepositories")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="grid gap-3">
          <div className="space-y-1.5" data-settings-dirty={orgsDirty}>
            <SettingsFieldLabel>{t("github:organizations")}</SettingsFieldLabel>
            <div data-testid="github-scope-orgs">
              <GitHubOrganizationMultiSelect
                workspaceId={workspaceId}
                selected={orgs}
                onChange={onOrgsChange}
                disabled={loading || mode !== "orgs"}
              />
            </div>
          </div>
          <div className="space-y-1.5" data-settings-dirty={reposDirty}>
            <SettingsFieldLabel>{t("github:repositories")}</SettingsFieldLabel>
            <div data-testid="github-scope-repos">
              <GitHubRepositoryMultiSelect
                workspaceId={workspaceId}
                selected={repos}
                onChange={onReposChange}
                repositorySource="scope-editor"
                disabled={loading || mode !== "repos"}
              />
            </div>
          </div>
        </div>
      </CardContent>
    </SettingsCard>
  );
}

function useGitHubRepoScopeDraft(workspaceId: string) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [draftHost, setDraftHost] = useState("github.com");
  const captureRequest = useConnectionRequestGuard(`${workspaceId}:${draftHost}`);
  const [mode, setMode] = useState<GitHubRepoScopeMode>("all");
  const [orgs, setOrgs] = useState<string[]>([]);
  const [repos, setRepos] = useState<RepoFilter[]>([]);
  const [baseline, setBaseline] = useState<ScopeDraft>({ mode: "all", orgs: [], repos: [] });
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    void fetchGitHubWorkspaceSettings(workspaceId)
      .then((settings) => {
        if (cancelled) return;
        const next = settingsScopeDraft(settings);
        setDraftHost(settings.github_host ?? "github.com");
        setBaseline(next);
        setMode(next.mode);
        setOrgs(next.orgs);
        setRepos(next.repos);
      })
      .catch(() => {
        // The module-level `t` resolves at call time, so this follows the active
        // locale without putting the hook's `t` in the effect deps — that would
        // refetch on every locale switch and overwrite the user's unsaved edits
        // via the setMode/setOrgs/setRepos calls above.
        if (!cancelled)
          toast({
            description: translate("github:failedToLoadGithubWorkspaceSettings"),
            variant: "error",
          });
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [toast, workspaceId]);

  const save = useCallback(async () => {
    const isCurrent = captureRequest();
    const submitted = { mode, orgs, repos };
    try {
      const updated = await updateGitHubWorkspaceSettings({
        ...scopePayload({ workspaceId, mode, orgs, repos }),
        expected_host: draftHost,
      });
      if (!isCurrent()) return;
      const saved = settingsScopeDraft(updated);
      setBaseline(saved);
      setMode((current) => (current === submitted.mode ? saved.mode : current));
      setOrgs((current) =>
        orgListKey(current) === orgListKey(submitted.orgs) ? saved.orgs : current,
      );
      setRepos((current) =>
        repoListKey(current) === repoListKey(submitted.repos) ? saved.repos : current,
      );
      toast({ description: t("github:githubWorkspaceSettingsSaved"), variant: "success" });
    } catch {
      if (!isCurrent()) return;
      toast({ description: t("github:failedToSaveGithubWorkspaceSettings"), variant: "error" });
      throw new Error("Failed to save GitHub workspace settings");
    }
  }, [draftHost, mode, orgs, repos, t, toast, workspaceId]);
  const discard = useCallback(() => {
    setMode(baseline.mode);
    setOrgs(baseline.orgs);
    setRepos(baseline.repos);
  }, [baseline]);
  const revision = draftRevision({ mode, orgs, repos });
  const dirty = revision !== draftRevision(baseline);

  useSettingsSaveContributor({
    id: `github-repo-scope:${workspaceId}`,
    revision,
    isDirty: dirty,
    canSave: !loading,
    save,
    discard,
  });

  return {
    mode,
    orgs,
    repos,
    baseline,
    loading,
    setMode,
    setOrgs,
    setRepos,
  };
}

export function GitHubRepoScopeSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const draft = useGitHubRepoScopeDraft(workspaceId);

  return (
    <SettingsSection
      title={t("github:repositoryScope")}
      titleAccessory={<RepositoryScopeHelp />}
      description={t("github:limitsGithubPullRequestsAndIssues")}
    >
      <RepositoryScopeFields
        workspaceId={workspaceId}
        mode={draft.mode}
        orgs={draft.orgs}
        repos={draft.repos}
        baseline={draft.baseline}
        loading={draft.loading}
        onModeChange={draft.setMode}
        onOrgsChange={draft.setOrgs}
        onReposChange={draft.setRepos}
      />
    </SettingsSection>
  );
}

function settingsScopeDraft(
  settings: Awaited<ReturnType<typeof fetchGitHubWorkspaceSettings>>,
): ScopeDraft {
  return {
    mode: settings.repo_scope_mode ?? "all",
    orgs: settings.repo_scope_orgs ?? [],
    repos: settings.repo_scope_repos ?? [],
  };
}

"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { Switch } from "@kandev/ui/switch";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useToast } from "@/components/toast-provider";
import { SettingsFieldLabel } from "@/components/settings/settings-typography";
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
    repo_scope_orgs: mode === "all" ? [] : orgs,
    repo_scope_repos: mode === "all" ? [] : repos,
  };
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
      <CardContent className="space-y-4 py-4">
        <div className="flex items-center gap-2" data-settings-dirty={mode !== baseline.mode}>
          <Switch
            id="github-scope-all-repositories"
            checked={mode === "all"}
            onCheckedChange={(checked) => onModeChange(checked ? "all" : "selected")}
            disabled={loading}
            data-testid="github-scope-all-toggle"
          />
          <SettingsFieldLabel
            htmlFor="github-scope-all-repositories"
            className="cursor-pointer font-normal max-md:min-h-11 [@media(any-pointer:coarse)]:min-h-11"
          >
            {t("github:allRepositories")}
          </SettingsFieldLabel>
        </div>
        {mode === "selected" && (
          <div className="grid gap-3 md:grid-cols-2">
            <div className="space-y-1.5" data-settings-dirty={orgsDirty}>
              <SettingsFieldLabel>{t("github:organizations")}</SettingsFieldLabel>
              <div data-testid="github-scope-orgs">
                <GitHubOrganizationMultiSelect
                  workspaceId={workspaceId}
                  selected={orgs}
                  onChange={onOrgsChange}
                  disabled={loading}
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
                  disabled={loading}
                />
              </div>
            </div>
          </div>
        )}
      </CardContent>
    </SettingsCard>
  );
}

function useGitHubRepoScopeDraft(workspaceId: string) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [draftHost, setDraftHost] = useState("github.com");
  const captureRequest = useConnectionRequestGuard(`${workspaceId}:${draftHost}`);
  const [mode, setModeState] = useState<GitHubRepoScopeMode>("all");
  const [orgs, setOrgsState] = useState<string[]>([]);
  const [repos, setReposState] = useState<RepoFilter[]>([]);
  const draftRef = useRef<ScopeDraft>({ mode: "all", orgs: [], repos: [] });
  const [baseline, setBaseline] = useState<ScopeDraft>({ mode: "all", orgs: [], repos: [] });
  const [loading, setLoading] = useState(true);

  const setMode = useCallback((next: GitHubRepoScopeMode) => {
    draftRef.current = { ...draftRef.current, mode: next };
    setModeState(next);
  }, []);
  const setOrgs = useCallback((next: string[]) => {
    draftRef.current = { ...draftRef.current, orgs: next };
    setOrgsState(next);
  }, []);
  const setRepos = useCallback((next: RepoFilter[]) => {
    draftRef.current = { ...draftRef.current, repos: next };
    setReposState(next);
  }, []);
  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    void fetchGitHubWorkspaceSettings(workspaceId)
      .then((settings) => {
        if (cancelled) return;
        const next = settingsScopeDraft(settings);
        setDraftHost(settings.github_host ?? "github.com");
        draftRef.current = next;
        setBaseline(next);
        setModeState(next.mode);
        setOrgsState(next.orgs);
        setReposState(next.repos);
      })
      .catch(() => {
        // Resolve at call time so locale changes do not rerun this effect and overwrite drafts.
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
    const submittedRevision = draftRevision({ mode, orgs, repos });
    try {
      const updated = await updateGitHubWorkspaceSettings({
        ...scopePayload({ workspaceId, mode, orgs, repos }),
        expected_host: draftHost,
      });
      if (!isCurrent()) return;
      const saved = settingsScopeDraft(updated);
      setBaseline(saved);
      if (draftRevision(draftRef.current) === submittedRevision) {
        setMode(saved.mode);
        setOrgs(saved.orgs);
        setRepos(saved.repos);
      }
      toast({ description: t("github:githubWorkspaceSettingsSaved"), variant: "success" });
    } catch {
      if (!isCurrent()) return;
      toast({ description: t("github:failedToSaveGithubWorkspaceSettings"), variant: "error" });
      throw new Error("Failed to save GitHub workspace settings");
    }
  }, [draftHost, mode, orgs, repos, setMode, setOrgs, setRepos, t, toast, workspaceId]);

  const discard = useCallback(() => {
    setMode(baseline.mode);
    setOrgs(baseline.orgs);
    setRepos(baseline.repos);
  }, [baseline, setMode, setOrgs, setRepos]);
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
      <div data-testid="github-repo-scope-section">
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
      </div>
    </SettingsSection>
  );
}

function settingsScopeDraft(
  settings: Awaited<ReturnType<typeof fetchGitHubWorkspaceSettings>>,
): ScopeDraft {
  return {
    mode: settings.repo_scope_mode === "all" ? "all" : "selected",
    orgs: settings.repo_scope_orgs ?? [],
    repos: settings.repo_scope_repos ?? [],
  };
}

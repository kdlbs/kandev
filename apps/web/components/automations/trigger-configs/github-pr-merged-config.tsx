"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { Input } from "@kandev/ui/input";
import { RepoFilterSelector } from "@/components/github/repo-filter-selector";
import type { RepoFilter } from "@/lib/types/github";

type GitHubPRMergedConfigProps = {
  config: Record<string, unknown>;
  workspaceId: string;
  onUpdate: (config: Record<string, unknown>) => void;
};

export function GitHubPRMergedConfig({ config, workspaceId, onUpdate }: GitHubPRMergedConfigProps) {
  const { t } = useTranslation();
  const allRepos = (config.all_repos as boolean) ?? false;
  const repos = (config.repos as RepoFilter[]) ?? [];
  const configBranches = ((config.base_branches as string[]) ?? []).join(", ");
  const [branches, setBranches] = useState(configBranches);
  useEffect(() => {
    setBranches(configBranches);
  }, [configBranches]);

  const commitBranches = (raw: string) => {
    const parsed = raw
      .split(",")
      .map((b) => b.trim())
      .filter(Boolean);
    onUpdate({ ...config, base_branches: parsed });
  };

  const deadConfig = !allRepos && repos.length === 0;

  return (
    <div className="space-y-3">
      <RepoFilterSelector
        allRepos={allRepos}
        selectedRepos={repos}
        workspaceId={workspaceId}
        onAllReposChange={(checked) => {
          onUpdate({ ...config, all_repos: checked, repos: checked ? [] : repos });
        }}
        onSelectedReposChange={(next) => onUpdate({ ...config, repos: next })}
      />
      {deadConfig && (
        <p className="text-xs text-destructive">{t("automations:prMergedDeadConfigWarning")}</p>
      )}
      <div className="space-y-1.5">
        <Label className="text-xs" htmlFor="github-pr-merged-base-branches">
          {t("automations:baseBranchesLabel")}
        </Label>
        {/* Example branch names — data the user types verbatim, not copy. */}
        <Input
          id="github-pr-merged-base-branches"
          value={branches}
          onChange={(e) => {
            setBranches(e.target.value);
            commitBranches(e.target.value);
          }}
          // eslint-disable-next-line i18next/no-literal-string -- example branch names, see above
          placeholder="main, release/*"
        />
      </div>
    </div>
  );
}

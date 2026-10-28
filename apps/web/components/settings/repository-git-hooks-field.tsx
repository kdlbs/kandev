import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { useFeature } from "@/hooks/domains/features/use-feature";
import type { Repository } from "@/lib/types/http";

type RepositoryGitHooksFieldProps = {
  repositoryId: string;
  savedRepository?: Repository;
  onUpdate: (repoId: string, updates: Partial<Repository>) => void;
  runGitHooks: boolean;
};

export function RepositoryGitHooksField({
  repositoryId,
  savedRepository,
  onUpdate,
  runGitHooks,
}: RepositoryGitHooksFieldProps) {
  const { t } = useTranslation();
  const switchId = `repo-run-git-hooks-${repositoryId}`;
  const inactiveId = `${switchId}-inactive`;
  const isolationEnabled = useFeature("managedGitIsolation");
  return (
    <div className="space-y-2" data-testid="repository-run-git-hooks">
      <div className="flex items-center gap-2">
        <Switch
          id={switchId}
          checked={runGitHooks}
          disabled={!isolationEnabled}
          aria-describedby={isolationEnabled ? undefined : inactiveId}
          onCheckedChange={(checked) => onUpdate(repositoryId, { run_git_hooks: checked })}
          data-testid="repository-run-git-hooks-switch"
          data-settings-dirty={runGitHooks !== (savedRepository?.run_git_hooks ?? false)}
        />
        <Label
          htmlFor={switchId}
          className="flex min-h-11 flex-1 cursor-pointer items-center md:min-h-0"
        >
          {t("workspaces:runGitHooks")}
        </Label>
      </div>
      <p className="text-sm text-muted-foreground">{t("workspaces:runGitHooksHelp")}</p>
      {!isolationEnabled && (
        <p
          id={inactiveId}
          className="text-sm text-muted-foreground"
          data-testid="repository-run-git-hooks-inactive"
        >
          {t("workspaces:runGitHooksInactive")}
        </p>
      )}
    </div>
  );
}

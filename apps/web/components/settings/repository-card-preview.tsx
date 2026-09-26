"use client";

import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { IconEdit, IconGitBranch, IconTrash } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { CardContent } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { UnsavedChangesBadge } from "@/components/settings/unsaved-indicator";
import { SettingsCard } from "@/components/settings/settings-card";
import type { Repository, RepositoryScript } from "@/lib/types/http";

type RepositoryWithScripts = Repository & { scripts: RepositoryScript[] };

type RepositoryPreviewProps = {
  repository: RepositoryWithScripts;
  isDirty: boolean;
  deleteLoading: boolean;
  onOpenDelete: () => void;
  open: () => void;
};

// These two build every string in the collapsed card, and neither holds JSX, so
// `i18next/no-literal-string` could never see any of it — the lint count for this
// file reported zero for the whole block. `t` is a parameter rather than the
// module-level import so each label resolves when the card renders.
function buildRepoScriptsSummary(t: TFunction, repository: RepositoryWithScripts) {
  const setupScript = repository.setup_script ?? "";
  const cleanupScript = repository.cleanup_script ?? "";
  const devScript = repository.dev_script ?? "";
  const scriptsCount = repository.scripts.length;
  const hasSetupScript = Boolean(setupScript.trim());
  const hasCleanupScript = Boolean(cleanupScript.trim());
  const hasDevScript = Boolean(devScript.trim());
  const showScriptsSummary = scriptsCount > 0 || hasSetupScript || hasCleanupScript || hasDevScript;
  // Was `custom script${count === 1 ? "" : "s"}` — an English morpheme built at
  // the call site, which leaves a translator no way to express a third form.
  const scriptsLabel =
    scriptsCount === 0
      ? t("workspaces:noCustomScripts")
      : t("workspaces:customScripts", { count: scriptsCount });
  return {
    scriptsCount,
    hasSetupScript,
    hasCleanupScript,
    hasDevScript,
    showScriptsSummary,
    scriptsLabel,
  };
}

function buildRepoPreviewData(t: TFunction, repository: RepositoryWithScripts) {
  const repositoryName = repository.name ?? "";
  // `source_type` is the wire value; only its badge label is copy.
  const sourceLabel =
    repository.source_type === "local" ? t("workspaces:sourceLocal") : t("workspaces:sourceRemote");
  // The path and the `owner/name` slug are user data / git metadata; only the
  // two "nothing recorded" fallbacks are copy.
  const subtitle =
    repository.source_type === "local"
      ? repository.local_path || t("workspaces:localPathNotSet")
      : [repository.provider_owner, repository.provider_name].filter(Boolean).join("/") ||
        repository.provider ||
        t("workspaces:remoteRepository");
  return {
    repositoryName,
    sourceLabel,
    subtitle,
    ...buildRepoScriptsSummary(t, repository),
  };
}

export function RepositoryPreview({
  repository,
  isDirty,
  deleteLoading,
  onOpenDelete,
  open,
}: RepositoryPreviewProps) {
  const { t } = useTranslation();
  const {
    repositoryName,
    scriptsCount,
    hasSetupScript,
    hasCleanupScript,
    hasDevScript,
    showScriptsSummary,
    scriptsLabel,
    sourceLabel,
    subtitle,
  } = buildRepoPreviewData(t, repository);

  return (
    <SettingsCard isDirty={isDirty}>
      <CardContent className="py-4 cursor-pointer" onClick={open}>
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-start gap-3 min-w-0">
            <div className="p-2 bg-muted rounded-md">
              <IconGitBranch className="h-4 w-4 text-muted-foreground" />
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2 flex-wrap">
                <h4 className="font-medium truncate">
                  {repositoryName || t("workspaces:untitledRepository")}
                </h4>
                <Badge variant="secondary" className="text-xs">
                  {sourceLabel}
                </Badge>
                {isDirty && <UnsavedChangesBadge />}
              </div>
              <div className="text-xs text-muted-foreground mt-1 truncate">{subtitle}</div>
              {showScriptsSummary ? (
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground mt-1">
                  {scriptsCount > 0 && <span>{scriptsLabel}</span>}
                  {hasSetupScript && <span>{t("workspaces:buildScriptChip")}</span>}
                  {hasCleanupScript && <span>{t("workspaces:cleanupScriptChip")}</span>}
                  {hasDevScript && <span>{t("workspaces:devScriptChip")}</span>}
                </div>
              ) : null}
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="cursor-pointer"
              onClick={(event) => {
                event.stopPropagation();
                open();
              }}
            >
              <IconEdit className="h-4 w-4" />
              {t("workspaces:edit")}
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="cursor-pointer"
              onClick={(event) => {
                event.stopPropagation();
                onOpenDelete();
              }}
              disabled={deleteLoading}
            >
              <IconTrash className="h-4 w-4" />
              {t("workspaces:delete")}
            </Button>
          </div>
        </div>
      </CardContent>
    </SettingsCard>
  );
}

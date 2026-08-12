"use client";

import { IconCloudDownload, IconGitCherryPick, IconGitMerge } from "@tabler/icons-react";
import {
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "@kandev/ui/dropdown-menu";
import { useTranslation } from "react-i18next";
import type { PerRepoStatus } from "./changes-panel-header";

export function PerRepoPullMenu({
  repoNames,
  perRepoStatus,
  onRepoPull,
  onRepoRebase,
  onRepoMerge,
  pullDisabled,
  repoDisplayName,
}: {
  repoNames: string[];
  perRepoStatus: PerRepoStatus[];
  onRepoPull: (repo: string) => void;
  onRepoRebase: (repo: string) => void;
  onRepoMerge: (repo: string) => void;
  pullDisabled?: boolean;
  repoDisplayName?: (repositoryName: string) => string | undefined;
}) {
  const { t } = useTranslation();
  const statusByName = new Map(perRepoStatus.map((s) => [s.repository_name, s]));
  return (
    <>
      {repoNames.map((repo, idx) => {
        const s = statusByName.get(repo);
        const behind = s?.pullBehind ?? 0;
        const label = repoDisplayName?.(repo) || repo || t("task:repository2");
        return (
          <div key={repo || "__no_repo__"}>
            {idx > 0 && <DropdownMenuSeparator />}
            <DropdownMenuLabel className="text-[10px] text-muted-foreground/70 uppercase tracking-wide flex items-center justify-between">
              <span className="truncate">{label}</span>
              {behind > 0 && (
                <span className="text-yellow-500 normal-case tracking-normal">
                  {t("task:behindCount", { behind })}
                </span>
              )}
            </DropdownMenuLabel>
            <DropdownMenuItem
              onClick={() => onRepoPull(repo)}
              className="cursor-pointer text-xs gap-2"
              disabled={pullDisabled}
              title={pullDisabled ? t("task:providerHistoryUnavailable") : undefined}
            >
              <IconCloudDownload className="h-3.5 w-3.5 text-muted-foreground" />
              {t("task:pull")}
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => onRepoRebase(repo)}
              className="cursor-pointer text-xs gap-2"
            >
              <IconGitCherryPick className="h-3.5 w-3.5 text-muted-foreground" />
              {t("task:rebase")}
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => onRepoMerge(repo)}
              className="cursor-pointer text-xs gap-2"
            >
              <IconGitMerge className="h-3.5 w-3.5 text-muted-foreground" />
              {t("task:merge")}
            </DropdownMenuItem>
          </div>
        );
      })}
    </>
  );
}

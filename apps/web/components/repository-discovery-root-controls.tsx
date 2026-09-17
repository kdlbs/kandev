"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { FolderPicker } from "@/components/folder-picker";
import { cn } from "@/lib/utils";
import type { DesktopDiscoveryRoot } from "@/lib/types/http";

export type RepositoryDiscoveryRootControlsProps = {
  className?: string;
  presentation?: "card" | "picker";
  isLoading: boolean;
  discoveryRoots: DesktopDiscoveryRoot[];
  failedRoots?: string[];
  showRootActions?: boolean;
  showRefresh?: boolean;
  homeConfirmationRequired: boolean;
  onChooseDiscoveryRoot: (path: string) => void;
  onRefreshDiscovery: () => void;
  onReconnectDiscoveryRoot: (oldPath: string, newPath: string) => void;
  onRemoveDiscoveryRoot: (path: string) => void;
};

function DiscoveryFailureNotice({
  failedRoots,
  isLoading,
  showRefresh,
  showRootActions,
  onRefreshDiscovery,
}: {
  failedRoots: string[];
  isLoading: boolean;
  showRefresh: boolean;
  showRootActions: boolean;
  onRefreshDiscovery: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="grid max-h-[min(16rem,calc(100dvh-2rem))] min-h-0 grid-rows-[auto_minmax(0,1fr)_auto] gap-2 overflow-hidden rounded border border-amber-500/40 bg-amber-500/10 p-2 text-xs"
      role="status"
      aria-live="polite"
      data-testid="discovery-failure"
    >
      <div className="space-y-2">
        <p className="font-medium">{t("workspaces:repositoryDiscoveryFailedTitle")}</p>
        <p>{t("workspaces:repositoryDiscoveryFailedDescription")}</p>
      </div>
      <ul
        className="min-h-0 min-w-0 max-h-[min(9rem,40dvh)] space-y-1 overflow-y-auto overscroll-contain"
        aria-label={t("workspaces:repositoryDiscoveryFailedRoots")}
      >
        {failedRoots.map((path) => (
          <li key={path} className="break-all font-mono" title={path}>
            {path}
          </li>
        ))}
      </ul>
      {!showRootActions && showRefresh && (
        <Button
          type="button"
          variant="outline"
          className="[@media(pointer:coarse)]:h-11"
          onClick={onRefreshDiscovery}
          disabled={isLoading}
          data-testid="discovery-failure-refresh"
        >
          {t("workspaces:refreshRepositories")}
        </Button>
      )}
    </div>
  );
}

function SavedDiscoveryRootList({
  discoveryRoots,
  onReconnectDiscoveryRoot,
  onRemoveDiscoveryRoot,
}: {
  discoveryRoots: DesktopDiscoveryRoot[];
  onReconnectDiscoveryRoot: (oldPath: string, newPath: string) => void;
  onRemoveDiscoveryRoot: (path: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      {discoveryRoots.map((root) => (
        <div
          key={root.id || root.path}
          className="flex min-w-0 flex-wrap items-center gap-2 rounded border border-border/50 p-2 text-xs"
        >
          <span className="min-w-0 flex-1 truncate font-mono" title={root.path}>
            {root.display_path || root.path}
          </span>
          {root.state === "reconnect_required" && (
            <FolderPicker
              value=""
              placeholder={t("workspaces:reconnectDiscoveryRoot")}
              onChange={(newPath) => onReconnectDiscoveryRoot(root.path, newPath)}
            />
          )}
          <Button
            type="button"
            variant="ghost"
            className="[@media(pointer:coarse)]:h-11"
            onClick={() => onRemoveDiscoveryRoot(root.path)}
          >
            {t("workspaces:removeDiscoveryRoot")}
          </Button>
        </div>
      ))}
    </>
  );
}

export function RepositoryDiscoveryRootControls({
  className,
  presentation = "card",
  isLoading,
  discoveryRoots,
  failedRoots = [],
  showRootActions = true,
  showRefresh = true,
  homeConfirmationRequired,
  onChooseDiscoveryRoot,
  onRefreshDiscovery,
  onReconnectDiscoveryRoot,
  onRemoveDiscoveryRoot,
}: RepositoryDiscoveryRootControlsProps) {
  const { t } = useTranslation();
  const visibleFailedRoots = failedRoots.filter(
    (path, index) =>
      failedRoots.indexOf(path) === index && !discoveryRoots.some((root) => root.path === path),
  );
  return (
    <div
      className={cn(
        presentation === "picker"
          ? "space-y-2 border-b border-border/60 bg-muted/20 p-2"
          : "space-y-2 rounded-md border border-border/60 p-3",
        className,
      )}
      data-testid="discovery-root-controls"
      data-presentation={presentation}
    >
      {presentation === "card" && showRootActions && (
        <div>
          <p className="text-sm font-medium">
            {t("workspaces:chooseFoldersToDiscoverRepositories")}
          </p>
          <p className="text-xs text-muted-foreground">
            {t("workspaces:chooseFoldersToDiscoverRepositoriesDescription")}
          </p>
        </div>
      )}
      {showRootActions && (
        <div className="flex flex-wrap items-center gap-2">
          <FolderPicker
            value=""
            placeholder={t("workspaces:chooseFoldersToDiscoverRepositories")}
            onChange={onChooseDiscoveryRoot}
          />
          {showRefresh && (
            <Button
              type="button"
              variant="outline"
              className="[@media(pointer:coarse)]:h-11"
              onClick={onRefreshDiscovery}
              disabled={isLoading}
            >
              {t("workspaces:refreshRepositories")}
            </Button>
          )}
        </div>
      )}
      {visibleFailedRoots.length > 0 && (
        <DiscoveryFailureNotice
          failedRoots={visibleFailedRoots}
          isLoading={isLoading}
          showRefresh={showRefresh}
          showRootActions={showRootActions}
          onRefreshDiscovery={onRefreshDiscovery}
        />
      )}
      {showRootActions && homeConfirmationRequired && (
        <div className="rounded border border-amber-500/40 bg-amber-500/10 p-2 text-xs">
          <p>{t("workspaces:homeDiscoveryConfirmationDescription")}</p>
          <div className="mt-2">
            <FolderPicker
              value=""
              placeholder={t("workspaces:continueHomeDiscovery")}
              onChange={onChooseDiscoveryRoot}
            />
          </div>
        </div>
      )}
      {showRootActions && (
        <SavedDiscoveryRootList
          discoveryRoots={discoveryRoots}
          onReconnectDiscoveryRoot={onReconnectDiscoveryRoot}
          onRemoveDiscoveryRoot={onRemoveDiscoveryRoot}
        />
      )}
    </div>
  );
}

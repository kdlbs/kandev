"use client";

import { useTranslation } from "react-i18next";
import { IconFolder, IconHome, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { FolderPicker } from "@/components/folder-picker";
import { cn } from "@/lib/utils";
import type { DesktopDiscoveryRoot } from "@/lib/types/http";

export type RepositoryDiscoveryRootControlsProps = {
  className?: string;
  presentation?: "card" | "picker" | "dialog";
  isLoading: boolean;
  discoveryRoots: DesktopDiscoveryRoot[];
  homeConfirmationRequired: boolean;
  isConfirmingHomeDiscovery?: boolean;
  onConfirmHomeDiscovery: () => void;
  onChooseDiscoveryRoot: (path: string) => void;
  onRefreshDiscovery: () => void;
  onReconnectDiscoveryRoot: (oldPath: string, newPath: string) => void;
  onRemoveDiscoveryRoot: (path: string) => void;
};

function isHomeRoot(root: DesktopDiscoveryRoot): boolean {
  return root.display_path === "~" || root.path === "~";
}

function presentationClassName(presentation: "card" | "picker" | "dialog"): string {
  if (presentation === "picker") {
    return "space-y-2.5 border-b border-border/60 bg-muted/20 p-2";
  }
  if (presentation === "dialog") {
    return "space-y-3";
  }
  return "space-y-3 rounded-md border border-border/60 p-3";
}

function SavedDiscoveryRootList({
  discoveryRoots,
  isLoading,
  onReconnectDiscoveryRoot,
  onRemoveDiscoveryRoot,
}: {
  discoveryRoots: DesktopDiscoveryRoot[];
  isLoading: boolean;
  onReconnectDiscoveryRoot: (oldPath: string, newPath: string) => void;
  onRemoveDiscoveryRoot: (path: string) => void;
}) {
  const { t } = useTranslation();

  if (discoveryRoots.length === 0) {
    if (isLoading) {
      return (
        <div
          className="flex items-center gap-2.5 rounded-md border border-border/60 bg-muted/20 px-3 py-3 text-xs"
          data-testid="discovery-roots-loading"
        >
          <IconLoader2 className="h-4 w-4 animate-spin text-muted-foreground shrink-0" />
          <div className="min-w-0 flex-1">
            <p className="font-medium text-foreground">{t("workspaces:addingScanFolder")}</p>
            <p className="text-[11px] text-muted-foreground">
              {t("workspaces:scanningRepositories")}
            </p>
          </div>
        </div>
      );
    }

    return (
      <div className="rounded-md border border-dashed border-border/70 p-4 text-center text-xs text-muted-foreground">
        {t("workspaces:repositoryDiscoveryNoRoots")}
      </div>
    );
  }

  return (
    <div className="space-y-1.5" data-testid="discovery-roots-list">
      {discoveryRoots.map((root) => {
        const isHome = isHomeRoot(root);
        const displayName = isHome
          ? t("workspaces:userHomeFolder")
          : root.display_path || root.path;
        return (
          <div
            key={root.id || root.path}
            className="flex min-w-0 items-center justify-between gap-3 rounded-md border border-border/60 bg-muted/20 px-3 py-2 text-xs"
          >
            <div className="flex min-w-0 items-center gap-2.5 flex-1">
              {isHome ? (
                <IconHome className="h-4 w-4 shrink-0 text-muted-foreground" />
              ) : (
                <IconFolder className="h-4 w-4 shrink-0 text-muted-foreground" />
              )}
              <div className="min-w-0 flex-1">
                <p className="truncate font-medium text-foreground">{displayName}</p>
                <p
                  className="truncate font-mono text-[11px] text-muted-foreground"
                  title={root.path}
                >
                  {root.path}
                </p>
              </div>
            </div>
            <div className="flex shrink-0 items-center gap-1.5">
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
                size="sm"
                className="h-7 max-md:min-h-11 max-md:h-11 [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:h-11 text-xs text-muted-foreground hover:text-destructive cursor-pointer"
                onClick={() => onRemoveDiscoveryRoot(root.path)}
              >
                {t("workspaces:removeDiscoveryRoot")}
              </Button>
            </div>
          </div>
        );
      })}
    </div>
  );
}

export function RepositoryDiscoveryRootControls({
  className,
  presentation = "card",
  isLoading,
  discoveryRoots,
  homeConfirmationRequired,
  isConfirmingHomeDiscovery = false,
  onConfirmHomeDiscovery,
  onChooseDiscoveryRoot,
  onRefreshDiscovery,
  onReconnectDiscoveryRoot,
  onRemoveDiscoveryRoot,
}: RepositoryDiscoveryRootControlsProps) {
  const { t } = useTranslation();
  return (
    <div
      className={cn(presentationClassName(presentation), className)}
      data-testid="discovery-root-controls"
      data-presentation={presentation}
    >
      {presentation === "card" && (
        <div className="space-y-0.5">
          <p className="text-sm font-medium">
            {t("workspaces:chooseFoldersToDiscoverRepositories")}
          </p>
          <p className="text-xs text-muted-foreground">
            {t("workspaces:chooseFoldersToDiscoverRepositoriesDescription")}
          </p>
        </div>
      )}
      <div className="flex flex-wrap items-center gap-2">
        <FolderPicker
          value=""
          placeholder={t("workspaces:addScanFolder")}
          onChange={onChooseDiscoveryRoot}
        />
        <Button
          type="button"
          variant="outline"
          className="cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          onClick={onRefreshDiscovery}
          disabled={isLoading}
        >
          <span className="inline-flex items-center gap-1.5">
            {isLoading && (
              <IconLoader2
                aria-hidden="true"
                className="h-3.5 w-3.5 animate-spin text-muted-foreground"
              />
            )}
            <span>{t("workspaces:refreshRepositories")}</span>
          </span>
        </Button>
        {isLoading && (
          <span className="sr-only" role="status">
            {t("workspaces:scanningRepositories")}
          </span>
        )}
      </div>
      {homeConfirmationRequired && (
        <div className="rounded-md border border-amber-500/40 bg-amber-500/10 p-2.5 text-xs">
          <p>{t("workspaces:homeDiscoveryConfirmationDescription")}</p>
          <div className="mt-2 flex items-center gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
              disabled={isLoading || isConfirmingHomeDiscovery}
              aria-busy={isConfirmingHomeDiscovery}
              onClick={onConfirmHomeDiscovery}
            >
              {t("workspaces:continueHomeDiscovery")}
            </Button>
            {isConfirmingHomeDiscovery && (
              <span className="sr-only" role="status">
                {t("common:loading")}
              </span>
            )}
          </div>
        </div>
      )}
      <SavedDiscoveryRootList
        discoveryRoots={discoveryRoots}
        isLoading={isLoading}
        onReconnectDiscoveryRoot={onReconnectDiscoveryRoot}
        onRemoveDiscoveryRoot={onRemoveDiscoveryRoot}
      />
    </div>
  );
}

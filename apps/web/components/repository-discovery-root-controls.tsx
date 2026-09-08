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
  homeConfirmationRequired: boolean;
  onChooseDiscoveryRoot: (path: string) => void;
  onRefreshDiscovery: () => void;
  onReconnectDiscoveryRoot: (oldPath: string, newPath: string) => void;
  onRemoveDiscoveryRoot: (path: string) => void;
};

export function RepositoryDiscoveryRootControls({
  className,
  presentation = "card",
  isLoading,
  discoveryRoots,
  homeConfirmationRequired,
  onChooseDiscoveryRoot,
  onRefreshDiscovery,
  onReconnectDiscoveryRoot,
  onRemoveDiscoveryRoot,
}: RepositoryDiscoveryRootControlsProps) {
  const { t } = useTranslation();
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
      {presentation === "card" && (
        <div>
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
          placeholder={t("workspaces:chooseFoldersToDiscoverRepositories")}
          onChange={onChooseDiscoveryRoot}
        />
        <Button
          type="button"
          variant="outline"
          className="[@media(pointer:coarse)]:h-11"
          onClick={onRefreshDiscovery}
          disabled={isLoading}
        >
          {t("workspaces:refreshRepositories")}
        </Button>
      </div>
      {homeConfirmationRequired && (
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
    </div>
  );
}

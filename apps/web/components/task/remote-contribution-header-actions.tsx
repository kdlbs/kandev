"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useTranslation } from "react-i18next";
import {
  remoteContributionActionPolicy,
  type RemoteContributionRelation,
} from "@/hooks/domains/session/remote-contribution-relation";
import { openExternalLink } from "@/lib/desktop/external-links";
import {
  type RemoteContributionResolutionTarget,
  type useRemoteContributionResolution,
  useRemoteContributionResolutionConfirmation,
} from "./use-remote-contribution-resolution";
import { RemoteContributionActionItems } from "./remote-contribution-action-items";
import { RemoteContributionResolutionDialog } from "./remote-contribution-resolution-dialog";

export function RemoteContributionHeaderActions({
  relation,
  resolution,
  resolutionTarget,
  prUrl,
  prNumber,
}: {
  relation?: RemoteContributionRelation;
  resolution?: ReturnType<typeof useRemoteContributionResolution>;
  resolutionTarget?: RemoteContributionResolutionTarget | null;
  prUrl?: string;
  prNumber?: number;
}) {
  const { t } = useTranslation();
  const confirmResolution = useRemoteContributionResolutionConfirmation(resolution);
  if (relation?.kind !== "diverged" || !resolution || !resolutionTarget) return null;
  const policy = remoteContributionActionPolicy(relation);
  return (
    <div className="flex items-center gap-1">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            size="icon"
            variant="ghost"
            className="h-11 w-11 cursor-pointer text-yellow-600 hover:bg-yellow-500/10 hover:text-yellow-600 md:h-6 md:w-6 dark:text-yellow-400 dark:hover:text-yellow-300"
            aria-label={t("task:remoteContributionChangedTitle")}
            title={t("task:remoteContributionChangedTooltip")}
            data-testid="header-remote-contribution-warning"
            disabled={resolution.isLoading}
          >
            <IconAlertTriangle className="h-4 w-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="end"
          className="w-80"
          data-testid="header-remote-contribution-menu"
        >
          <DropdownMenuLabel className="whitespace-normal px-2 py-2">
            <div className="flex items-start gap-2">
              <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-yellow-500" />
              <div className="space-y-1">
                <p className="font-medium text-foreground">
                  {t("task:remoteContributionChangedTitle")}
                </p>
                <p className="font-normal leading-relaxed text-muted-foreground">
                  {t("task:remoteContributionChangedBody")}
                </p>
              </div>
            </div>
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          <RemoteContributionActionItems
            disabled={resolution.isLoading}
            replaceDisabled={policy.replaceDisabled}
            useDisabled={policy.useDisabled}
            onReplaceContribution={() => resolution.requestReplace(resolutionTarget)}
            onUseContribution={() => resolution.requestUse(resolutionTarget)}
            onViewPRVersion={
              prUrl ? () => void openExternalLink(prUrl).catch(() => undefined) : undefined
            }
            testIdPrefix="header"
            prNumber={prNumber}
          />
        </DropdownMenuContent>
      </DropdownMenu>
      {resolution.pending && (
        <RemoteContributionResolutionDialog
          open
          action={resolution.pending.action}
          repositoryName={resolutionTarget.repositoryName ?? ""}
          expectedRemoteHead={resolution.pending.expectedRemoteHead}
          isLoading={resolution.isLoading}
          errorKey={resolution.errorKey}
          onOpenChange={(open) => {
            if (!open) resolution.cancel();
          }}
          onConfirm={() => {
            void confirmResolution();
          }}
        />
      )}
    </div>
  );
}

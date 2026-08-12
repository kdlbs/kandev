"use client";

import { IconAlertTriangle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import {
  remoteContributionActionPolicy,
  type RemoteContributionRelation,
} from "@/hooks/domains/session/remote-contribution-relation";
import {
  type RemoteContributionResolutionTarget,
  type useRemoteContributionResolution,
} from "./use-remote-contribution-resolution";
import { RemoteContributionResolutionDialog } from "./remote-contribution-resolution-dialog";

export function RemoteContributionHeaderActions({
  relation,
  resolution,
  resolutionTarget,
}: {
  relation?: RemoteContributionRelation;
  resolution?: ReturnType<typeof useRemoteContributionResolution>;
  resolutionTarget?: RemoteContributionResolutionTarget | null;
}) {
  const { t } = useTranslation();
  if (relation?.kind !== "diverged" || !resolution || !resolutionTarget) return null;
  const policy = remoteContributionActionPolicy(relation);
  return (
    <div className="flex items-center gap-1">
      <Button
        type="button"
        size="sm"
        variant="destructive"
        className="h-5 px-1.5 text-[11px] gap-1 cursor-pointer"
        data-testid="header-replace-pr-branch"
        disabled={policy.replaceDisabled || resolution.isLoading}
        onClick={() => resolution.requestReplace(resolutionTarget)}
      >
        <IconAlertTriangle className="h-3 w-3" />
        {t("task:replacePRBranch")}
      </Button>
      <Button
        type="button"
        size="sm"
        variant="ghost"
        className="h-5 px-1.5 text-[11px] cursor-pointer"
        data-testid="header-use-pr-version"
        disabled={policy.useDisabled || resolution.isLoading}
        onClick={() => resolution.requestUse(resolutionTarget)}
      >
        {t("task:usePRVersion")}
      </Button>
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
            void resolution.confirm();
          }}
        />
      )}
    </div>
  );
}

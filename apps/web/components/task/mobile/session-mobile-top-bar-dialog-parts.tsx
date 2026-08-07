"use client";

import { IconGitPullRequest, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Trans, useTranslation } from "react-i18next";

export function MobilePRBranchSummary({
  displayBranch,
  baseBranch,
  terminology,
}: {
  displayBranch: string | undefined;
  baseBranch: string | undefined;
  terminology: { shortName: string };
}) {
  return (
    <div className="text-sm text-muted-foreground">
      {baseBranch ? (
        <span>
          <Trans
            i18nKey="task:creatingFromTo"
            values={{ shortName: terminology.shortName, displayBranch, baseBranch }}
          >
            Creating {{ shortName: terminology.shortName }} from{" "}
            <span className="font-medium text-foreground">{displayBranch}</span> to{" "}
            <span className="font-medium text-foreground">{baseBranch}</span>
          </Trans>
        </span>
      ) : (
        <span>
          <Trans
            i18nKey="task:creatingFrom"
            values={{ shortName: terminology.shortName, displayBranch }}
          >
            Creating {{ shortName: terminology.shortName }} from{" "}
            <span className="font-medium text-foreground">{displayBranch}</span>
          </Trans>
        </span>
      )}
    </div>
  );
}

export function CommitSummary({
  uncommittedCount,
  uncommittedAdditions,
  uncommittedDeletions,
}: {
  uncommittedCount: number;
  uncommittedAdditions: number;
  uncommittedDeletions: number;
}) {
  const { t } = useTranslation();
  if (uncommittedCount <= 0) return <span>{t("task:noChangesToCommit")}</span>;
  return (
    <span>
      <Trans
        i18nKey="task:filesChanged"
        count={uncommittedCount}
        values={{ count: uncommittedCount }}
      >
        <span className="font-medium text-foreground">{uncommittedCount}</span> files changed
      </Trans>
      {(uncommittedAdditions > 0 || uncommittedDeletions > 0) && (
        <span className="ml-2">
          (<span className="text-green-600">+{uncommittedAdditions}</span>
          {" / "}
          <span className="text-red-600">-{uncommittedDeletions}</span>)
        </span>
      )}
    </span>
  );
}

export function PRSubmitButton({
  prTitle,
  prBody,
  prDraft,
  isGitLoading,
  onCreatePR,
  terminology,
  branchPushed,
}: {
  prTitle: string;
  prBody: string;
  prDraft: boolean;
  isGitLoading: boolean;
  onCreatePR: (title: string, body: string, draft: boolean) => void;
  terminology: { shortName: string };
  branchPushed: boolean;
}) {
  const { t } = useTranslation();
  return (
    <Button
      onClick={() => onCreatePR(prTitle.trim(), prBody.trim(), prDraft)}
      disabled={!prTitle.trim() || isGitLoading}
      className="bg-cyan-600 hover:bg-cyan-700 text-white"
    >
      {isGitLoading ? (
        <>
          <IconLoader2 className="h-4 w-4 animate-spin mr-2" />
          {t("task:creatingEllipsis")}
        </>
      ) : (
        <>
          <IconGitPullRequest className="h-4 w-4 mr-2" />
          {branchPushed
            ? t("task:retryChangeRequest", { shortName: terminology.shortName })
            : t("task:createChangeRequest", { shortName: terminology.shortName })}
        </>
      )}
    </Button>
  );
}

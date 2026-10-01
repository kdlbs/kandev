"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import type { TaskPR } from "@/lib/types/github";
import type { RemoteContributionRelation } from "@/hooks/domains/session/remote-contribution-relation";
import {
  remoteContributionActionPolicy,
  remoteContributionActionReasonKey,
} from "@/hooks/domains/session/remote-contribution-relation";
import {
  buildRemoteContributionResolutionTarget,
  type RemoteContributionResolutionTarget,
} from "./use-remote-contribution-resolution";

export function useChangesPanelContributionState(
  relation: RemoteContributionRelation,
  repositoryScope: string,
  selectedPR: TaskPR | null | undefined,
) {
  const { t } = useTranslation();
  const remoteRepositoryLabel = t("task:remoteRepository");
  const resolutionTarget = useMemo(
    () =>
      buildRemoteContributionResolutionTarget(
        relation,
        repositoryScope,
        selectedPR,
        remoteRepositoryLabel,
      ),
    [relation, repositoryScope, selectedPR, remoteRepositoryLabel],
  );
  const remoteActionPolicy = useMemo(() => remoteContributionActionPolicy(relation), [relation]);
  const pullDisabledReason = useMemo(() => {
    const key = remoteContributionActionReasonKey(relation, "pull");
    return key ? t(key) : undefined;
  }, [relation, t]);
  return { resolutionTarget, remoteActionPolicy, pullDisabledReason };
}

export type { RemoteContributionResolutionTarget };

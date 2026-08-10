export type RemoteContributionRelationKind =
  | "not_applicable"
  | "aligned"
  | "local_ahead"
  | "provider_ahead"
  | "diverged"
  | "unknown";

export type RemoteContributionPresentation = "unified" | "separate";

export type RemoteContributionRelationInput = {
  hasSelectedPR: boolean;
  providerCommits: ReadonlyArray<{ sha: string }>;
  providerLoading: boolean;
  providerError: string | null;
  localHead: string | null | undefined;
  upstreamHead: string | null | undefined;
  remoteAhead: number;
  remoteBehind: number;
  baseAhead: number;
  hasUpstream: boolean;
};

export type RemoteContributionRelation = {
  kind: RemoteContributionRelationKind;
  presentation: RemoteContributionPresentation;
  providerHead: string | null;
  pushAhead: number;
  pullBehind: number;
  canPush: boolean;
  canPull: boolean;
  remoteMutationBlocked: boolean;
};

export type RemoteContributionActionPolicy = {
  pushDisabled: boolean;
  pullDisabled: boolean;
  disabledReason: "history_changed" | null;
};

/**
 * Safety gates shared by desktop and mobile Git controls. A provider-ahead
 * checkout may pull, but must not push over the provider's newer history.
 * Diverged histories require reconciliation outside these controls.
 */
export function remoteContributionActionPolicy(
  relation: RemoteContributionRelation,
): RemoteContributionActionPolicy {
  const historyChanged = relation.remoteMutationBlocked;
  return {
    pushDisabled: historyChanged || relation.kind === "provider_ahead",
    pullDisabled: historyChanged,
    disabledReason: historyChanged ? "history_changed" : null,
  };
}

function nonNegative(value: number): number {
  return Math.max(0, value);
}

function fallbackCapabilities(input: RemoteContributionRelationInput) {
  const pushAhead = input.hasUpstream
    ? nonNegative(input.remoteAhead)
    : nonNegative(input.baseAhead);
  const pullBehind = input.hasUpstream ? nonNegative(input.remoteBehind) : 0;
  return {
    pushAhead,
    pullBehind,
    canPush: pushAhead > 0,
    canPull: pullBehind > 0,
    remoteMutationBlocked: false,
  };
}

function result(
  kind: RemoteContributionRelationKind,
  providerHead: string | null,
  capabilities: ReturnType<typeof fallbackCapabilities>,
): RemoteContributionRelation {
  return {
    kind,
    presentation: kind === "diverged" ? "separate" : "unified",
    providerHead,
    ...capabilities,
  };
}

export function classifyRemoteContribution(
  input: RemoteContributionRelationInput,
): RemoteContributionRelation {
  const fallback = fallbackCapabilities(input);
  if (!input.hasSelectedPR) {
    return result("not_applicable", null, fallback);
  }

  const providerHead = input.providerCommits.at(-1)?.sha ?? null;
  const evidenceUnavailable =
    input.providerLoading ||
    Boolean(input.providerError) ||
    input.providerCommits.length === 0 ||
    !providerHead ||
    !input.localHead ||
    !input.upstreamHead;
  if (evidenceUnavailable) {
    return result("unknown", providerHead, fallback);
  }

  if (input.localHead === providerHead) {
    return result("aligned", providerHead, fallback);
  }

  if (input.providerCommits.some((commit) => commit.sha === input.localHead)) {
    return result("provider_ahead", providerHead, {
      ...fallback,
      canPush: false,
      canPull: true,
      remoteMutationBlocked: false,
    });
  }

  if (input.upstreamHead === providerHead && input.remoteAhead > 0 && input.remoteBehind === 0) {
    return result("local_ahead", providerHead, {
      ...fallback,
      pushAhead: input.remoteAhead,
      canPush: true,
      canPull: false,
      remoteMutationBlocked: false,
    });
  }

  return result("diverged", providerHead, {
    ...fallback,
    canPush: false,
    canPull: false,
    remoteMutationBlocked: true,
  });
}

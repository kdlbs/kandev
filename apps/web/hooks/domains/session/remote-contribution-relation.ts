export type RemoteContributionRelationKind =
  | "not_applicable"
  | "aligned"
  | "local_ahead"
  | "provider_ahead"
  | "diverged"
  | "unknown";

export type RemoteContributionPresentation = "unified" | "separate";

export type RemoteContributionAction =
  | "normal_push"
  | "provider_ahead_pull"
  | "diverged_replace"
  | "unavailable_evidence";

export type RemoteContributionRelationInput = {
  hasSelectedPR: boolean;
  providerCommits: ReadonlyArray<{ sha: string }>;
  providerHead: string | null | undefined;
  providerCommitsComplete: boolean;
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
  action: RemoteContributionAction;
  providerHead: string | null;
  pushAhead: number;
  pullBehind: number;
  canPush: boolean;
  canPull: boolean;
  canReplaceRemote: boolean;
  canUseRemote: boolean;
};

export type RemoteContributionActionPolicy = {
  action: RemoteContributionAction;
  pushDisabled: boolean;
  pullDisabled: boolean;
  replaceDisabled: boolean;
  useDisabled: boolean;
  disabledReason: "provider_evidence_unavailable" | null;
};

export type RemoteContributionActionReasonKey =
  | "task:providerAheadPushDisabled"
  | "task:providerAheadPullRequiresUpstream"
  | "task:divergedActionsUnavailable";

export function remoteContributionActionReasonKey(
  relation: RemoteContributionRelation,
  action: "push" | "pull",
): RemoteContributionActionReasonKey | null {
  if (relation.action === "provider_ahead_pull") {
    if (action === "push") return "task:providerAheadPushDisabled";
    if (!relation.canPull) return "task:providerAheadPullRequiresUpstream";
  }
  if (relation.action === "diverged_replace") return "task:divergedActionsUnavailable";
  return null;
}

/**
 * Safety gates shared by desktop and mobile Git controls. A provider-ahead
 * checkout may pull, but must not push over the provider's newer history.
 * Diverged histories expose explicit contribution replacement or adoption.
 */
export function remoteContributionActionPolicy(
  relation: RemoteContributionRelation,
): RemoteContributionActionPolicy {
  const unavailable = relation.action === "unavailable_evidence";
  const diverged = relation.action === "diverged_replace";
  return {
    action: relation.action,
    pushDisabled: diverged || relation.action === "provider_ahead_pull",
    pullDisabled: diverged || (relation.action === "provider_ahead_pull" && !relation.canPull),
    replaceDisabled: !relation.canReplaceRemote,
    useDisabled: !relation.canUseRemote,
    disabledReason: unavailable ? "provider_evidence_unavailable" : null,
  };
}

export function isFullCommitSHA(value: string | null | undefined): boolean {
  return (
    (value?.length === 40 || value?.length === 64) &&
    [...(value ?? "")].every((character) => /^[0-9a-f]$/i.test(character))
  );
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
    canReplaceRemote: false,
    canUseRemote: false,
  };
}

function actionFor(
  kind: RemoteContributionRelationKind,
  providerHead: string | null,
): RemoteContributionAction {
  if (kind === "provider_ahead") return "provider_ahead_pull";
  if (kind === "diverged" && isFullCommitSHA(providerHead)) return "diverged_replace";
  if (kind === "unknown") return "unavailable_evidence";
  if (kind === "diverged") return "unavailable_evidence";
  return "normal_push";
}

function result(
  kind: RemoteContributionRelationKind,
  providerHead: string | null,
  capabilities: ReturnType<typeof fallbackCapabilities>,
): RemoteContributionRelation {
  const action = actionFor(kind, providerHead);
  return {
    kind,
    presentation: kind === "diverged" ? "separate" : "unified",
    action,
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

  // The provider API supplies the head separately because a commit page can
  // be incomplete or ordered independently from the live branch head.
  const providerHead = input.providerHead ?? null;
  const evidenceUnavailable =
    input.providerLoading ||
    Boolean(input.providerError) ||
    !input.providerCommitsComplete ||
    input.providerCommits.length === 0 ||
    !providerHead ||
    !input.localHead;
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
      canPull: input.hasUpstream,
    });
  }

  if (!input.upstreamHead) {
    return result("unknown", providerHead, fallback);
  }

  if (input.upstreamHead === providerHead && input.remoteAhead > 0 && input.remoteBehind === 0) {
    return result("local_ahead", providerHead, {
      ...fallback,
      pushAhead: input.remoteAhead,
      canPush: true,
      canPull: false,
    });
  }

  const canResolve = isFullCommitSHA(providerHead);
  return result("diverged", providerHead, {
    ...fallback,
    canPush: false,
    canPull: false,
    canReplaceRemote: canResolve,
    canUseRemote: canResolve,
  });
}

import { describe, expect, it } from "vitest";
import {
  classifyRemoteContribution,
  remoteContributionActionReasonKey,
  remoteContributionActionPolicy,
  type RemoteContributionRelationInput,
} from "./remote-contribution-relation";

const LOCAL_HEAD = "local-head";
const PROVIDER_BASE = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
const PROVIDER_HEAD = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";
const REWRITTEN_BASE = "cccccccccccccccccccccccccccccccccccccccc";
const REWRITTEN_HEAD = "dddddddddddddddddddddddddddddddddddddddd";

const baseInput: RemoteContributionRelationInput = {
  hasSelectedPR: true,
  providerCommits: [{ sha: PROVIDER_BASE }, { sha: PROVIDER_HEAD }],
  providerHead: PROVIDER_HEAD,
  providerCommitsComplete: true,
  providerLoading: false,
  providerError: null,
  localHead: LOCAL_HEAD,
  upstreamHead: PROVIDER_HEAD,
  remoteAhead: 0,
  remoteBehind: 0,
  baseAhead: 4,
  hasUpstream: true,
};

function input(overrides: Partial<RemoteContributionRelationInput> = {}) {
  return { ...baseInput, ...overrides };
}

const classificationCases = [
  {
    name: "not applicable without a selected PR",
    overrides: { hasSelectedPR: false, hasUpstream: false, baseAhead: 2 },
    expected: { kind: "not_applicable", canPush: true, pushAhead: 2 },
  },
  {
    name: "aligned heads",
    overrides: { localHead: PROVIDER_HEAD },
    expected: { kind: "aligned", canPush: false, canPull: false },
  },
  {
    name: "local ahead of the current provider head",
    overrides: { remoteAhead: 1 },
    expected: { kind: "local_ahead", canPush: true, pushAhead: 1 },
  },
  {
    name: "provider ahead while retaining local HEAD",
    overrides: {
      providerCommits: [{ sha: PROVIDER_BASE }, { sha: LOCAL_HEAD }, { sha: PROVIDER_HEAD }],
      upstreamHead: PROVIDER_HEAD,
      remoteBehind: 1,
    },
    expected: { kind: "provider_ahead", canPush: false, canPull: true },
  },
  {
    name: "provider ahead without a configured upstream",
    overrides: {
      providerCommits: [{ sha: PROVIDER_BASE }, { sha: LOCAL_HEAD }, { sha: PROVIDER_HEAD }],
      upstreamHead: null,
      hasUpstream: false,
      baseAhead: 0,
    },
    expected: { kind: "provider_ahead", canPush: false, canPull: false },
  },
  {
    name: "rewritten provider history",
    overrides: {
      providerCommits: [{ sha: REWRITTEN_BASE }, { sha: REWRITTEN_HEAD }],
      upstreamHead: REWRITTEN_HEAD,
      remoteAhead: 2,
      remoteBehind: 2,
    },
    expected: {
      kind: "diverged",
      canPush: false,
      canPull: false,
      canReplaceRemote: true,
      canUseRemote: true,
      presentation: "separate",
    },
  },
];

const unknownCases = [
  { name: "provider commits are loading", providerLoading: true },
  { name: "provider commits failed", providerError: "provider unavailable" },
  { name: "provider commits are unexpectedly empty", providerCommits: [] },
  { name: "local HEAD is unavailable", localHead: null },
  { name: "upstream evidence is unavailable", upstreamHead: null, hasUpstream: true },
];

describe("classifyRemoteContribution", () => {
  it.each(classificationCases)("classifies $name", ({ overrides, expected }) => {
    expect(classifyRemoteContribution(input(overrides))).toMatchObject(expected);
  });

  it.each(unknownCases)("returns unknown when $name", (overrides) => {
    expect(classifyRemoteContribution(input(overrides))).toMatchObject({
      kind: "unknown",
      presentation: "unified",
      canPush: false,
      canPull: false,
      canReplaceRemote: false,
      canUseRemote: false,
      action: "unavailable_evidence",
    });
  });

  it("uses upstream counts instead of base-ahead for an existing upstream", () => {
    expect(
      classifyRemoteContribution(
        input({
          providerLoading: true,
          baseAhead: 12,
          hasUpstream: true,
          remoteAhead: 0,
          remoteBehind: 1,
        }),
      ),
    ).toMatchObject({ canPush: false, canPull: true, pushAhead: 0, pullBehind: 1 });
  });

  it("uses the first-push base fallback only without an upstream", () => {
    expect(
      classifyRemoteContribution(
        input({
          providerLoading: true,
          hasUpstream: false,
          upstreamHead: null,
          baseAhead: 3,
          remoteAhead: 0,
        }),
      ),
    ).toMatchObject({ canPush: true, pushAhead: 3, canPull: false, pullBehind: 0 });
  });

  it("does not equate equal metadata with equal commit identity", () => {
    expect(
      classifyRemoteContribution(
        input({
          providerCommits: [{ sha: "different-sha" }],
          upstreamHead: "different-sha",
        }),
      ),
    ).toMatchObject({ kind: "diverged", presentation: "separate" });
  });

  it("does not call an upstream mismatch local-ahead", () => {
    expect(
      classifyRemoteContribution(
        input({ upstreamHead: "unrelated-upstream", remoteAhead: 3, remoteBehind: 0 }),
      ),
    ).toMatchObject({
      kind: "diverged",
      canPush: false,
      canReplaceRemote: true,
      canUseRemote: true,
    });
  });

  it("uses the explicit provider head instead of the last listed commit", () => {
    const providerHeadInput = {
      ...input({
        providerCommits: [{ sha: PROVIDER_BASE }],
        localHead: PROVIDER_HEAD,
        upstreamHead: PROVIDER_HEAD,
      }),
      providerHead: PROVIDER_HEAD,
      providerCommitsComplete: true,
    };

    expect(classifyRemoteContribution(providerHeadInput)).toMatchObject({
      kind: "aligned",
      providerHead: PROVIDER_HEAD,
    });
  });

  it("returns unknown when provider ancestry data is incomplete", () => {
    const incompleteInput = {
      ...input({
        providerCommits: [{ sha: REWRITTEN_BASE }, { sha: REWRITTEN_HEAD }],
        upstreamHead: REWRITTEN_HEAD,
        remoteAhead: 2,
        remoteBehind: 2,
      }),
      providerHead: REWRITTEN_HEAD,
      providerCommitsComplete: false,
    };

    expect(classifyRemoteContribution(incompleteInput)).toMatchObject({
      kind: "unknown",
      providerHead: REWRITTEN_HEAD,
      action: "unavailable_evidence",
    });
  });
});

describe("remoteContributionActionPolicy", () => {
  it("offers explicit version choices for diverged histories", () => {
    const relation = classifyRemoteContribution(
      input({
        providerCommits: [{ sha: REWRITTEN_HEAD }],
        upstreamHead: REWRITTEN_HEAD,
        remoteAhead: 1,
        remoteBehind: 1,
      }),
    );

    expect(remoteContributionActionPolicy(relation)).toEqual({
      action: "diverged_replace",
      pushDisabled: true,
      pullDisabled: true,
      replaceDisabled: false,
      useDisabled: false,
      disabledReason: null,
    });
  });

  it("keeps Pull available for provider-ahead and blocks Push", () => {
    const relation = classifyRemoteContribution(
      input({
        providerCommits: [{ sha: LOCAL_HEAD }, { sha: PROVIDER_HEAD }],
        remoteBehind: 1,
      }),
    );

    expect(remoteContributionActionPolicy(relation)).toEqual({
      action: "provider_ahead_pull",
      pushDisabled: true,
      pullDisabled: false,
      replaceDisabled: true,
      useDisabled: true,
      disabledReason: null,
    });
  });

  it("does not offer Pull for provider-ahead without an upstream", () => {
    const relation = classifyRemoteContribution(
      input({
        providerCommits: [{ sha: LOCAL_HEAD }, { sha: PROVIDER_HEAD }],
        upstreamHead: null,
        hasUpstream: false,
      }),
    );

    expect(remoteContributionActionPolicy(relation)).toMatchObject({
      action: "provider_ahead_pull",
      pushDisabled: true,
      pullDisabled: true,
    });
  });

  it("returns action-specific disabled reason keys", () => {
    const providerAhead = classifyRemoteContribution(
      input({
        providerCommits: [{ sha: LOCAL_HEAD }, { sha: PROVIDER_HEAD }],
        upstreamHead: null,
        hasUpstream: false,
      }),
    );
    const diverged = classifyRemoteContribution(
      input({
        providerCommits: [{ sha: REWRITTEN_HEAD }],
        upstreamHead: REWRITTEN_HEAD,
        remoteAhead: 1,
        remoteBehind: 1,
      }),
    );

    expect(remoteContributionActionReasonKey(providerAhead, "push")).toBe(
      "task:providerAheadPushDisabled",
    );
    expect(remoteContributionActionReasonKey(providerAhead, "pull")).toBe(
      "task:providerAheadPullRequiresUpstream",
    );
    expect(remoteContributionActionReasonKey(diverged, "pull")).toBe(
      "task:divergedActionsUnavailable",
    );
  });

  it("keeps normal push behavior for local-ahead history", () => {
    const relation = classifyRemoteContribution(input({ remoteAhead: 2 }));

    expect(remoteContributionActionPolicy(relation)).toEqual({
      action: "normal_push",
      pushDisabled: false,
      pullDisabled: false,
      replaceDisabled: true,
      useDisabled: true,
      disabledReason: null,
    });
  });

  it("does not offer destructive choices when provider evidence is unavailable", () => {
    const relation = classifyRemoteContribution(input({ providerLoading: true }));

    expect(remoteContributionActionPolicy(relation)).toEqual({
      action: "unavailable_evidence",
      pushDisabled: false,
      pullDisabled: false,
      replaceDisabled: true,
      useDisabled: true,
      disabledReason: "provider_evidence_unavailable",
    });
  });
});

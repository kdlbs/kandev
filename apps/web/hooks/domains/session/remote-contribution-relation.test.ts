import { describe, expect, it } from "vitest";
import {
  classifyRemoteContribution,
  remoteContributionActionPolicy,
  type RemoteContributionRelationInput,
} from "./remote-contribution-relation";

const LOCAL_HEAD = "local-head";
const PROVIDER_BASE = "provider-base";
const PROVIDER_HEAD = "provider-head";
const REWRITTEN_BASE = "rewritten-base";
const REWRITTEN_HEAD = "rewritten-head";

const baseInput: RemoteContributionRelationInput = {
  hasSelectedPR: true,
  providerCommits: [{ sha: PROVIDER_BASE }, { sha: PROVIDER_HEAD }],
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
      remoteMutationBlocked: true,
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
      remoteMutationBlocked: false,
      canPush: false,
      canPull: false,
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
    ).toMatchObject({ kind: "diverged", canPush: false, remoteMutationBlocked: true });
  });
});

describe("remoteContributionActionPolicy", () => {
  it("blocks both remote mutations only for diverged histories", () => {
    const relation = classifyRemoteContribution(
      input({
        providerCommits: [{ sha: REWRITTEN_HEAD }],
        upstreamHead: REWRITTEN_HEAD,
        remoteAhead: 1,
        remoteBehind: 1,
      }),
    );

    expect(remoteContributionActionPolicy(relation)).toEqual({
      pushDisabled: true,
      pullDisabled: true,
      disabledReason: "history_changed",
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
      pushDisabled: true,
      pullDisabled: false,
      disabledReason: null,
    });
  });
});

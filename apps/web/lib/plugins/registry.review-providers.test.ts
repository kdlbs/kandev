import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "./registry";
import type { ReviewItemSummary, ReviewProviderRegistration, ReviewTaskAssociation } from "./types";

const PRIMARY_PLUGIN_ID = "plugin-a";
const SECONDARY_PLUGIN_ID = "plugin-b";
const SOURCE_CONTROL_PROVIDER_ID = "source-control";
const CHANGE_URL = "https://example.test/changes/1";
const REPOSITORY_ID = "repository-a";
const WORKSPACE_ID = "workspace-a";
const TASK_ID = "task-a";
const REVIEW_KEY = "change-1";
const CONNECTION_SCOPE = "https://example.test";
const IMMUTABLE_REPOSITORY_ID = "repository-immutable";

function reviewProvider(
  id: string,
  overrides: Partial<ReviewProviderRegistration> = {},
): ReviewProviderRegistration {
  return {
    id,
    label: id,
    changeRequestNoun: "change request",
    order: 0,
    getSnapshot: () => [],
    subscribe: () => () => {},
    refresh: async () => {},
    ReviewPanel: () => null,
    ...overrides,
  };
}

function providerSpecificStatusSnapshot() {
  return [
    {
      providerId: SOURCE_CONTROL_PROVIDER_ID,
      reviewKey: REVIEW_KEY,
      title: "Change 1",
      url: CHANGE_URL,
      connectionScope: CONNECTION_SCOPE,
      repositoryId: REPOSITORY_ID,
      changeRequestNumber: 1,
      state: "open",
      taskStatus: {
        number: 1,
        state: "open" as const,
        pipelineState: "failure" as const,
        checks: [
          {
            id: "build",
            label: "Build",
            state: "failure" as const,
            detail: "Failed in 2m",
            url: "https://example.test/checks/build",
            providerPayload: "discard",
          },
        ],
        review: {
          state: "approved" as const,
          approved: 1,
          required: 2,
          requested: 1,
          providerPayload: "discard",
        },
        unresolvedComments: 3,
        providerPayload: "discard",
      },
    },
  ];
}

function normalizedStatusSnapshot() {
  const [{ taskStatus, ...review }] = providerSpecificStatusSnapshot();
  const [{ providerPayload: _checkPayload, ...check }] = taskStatus.checks;
  const { providerPayload: _reviewPayload, ...reviewSummary } = taskStatus.review;
  const {
    providerPayload: _statusPayload,
    checks: _checks,
    review: _review,
    ...status
  } = taskStatus;
  return [
    {
      ...review,
      taskStatus: { ...status, checks: [check], review: reviewSummary },
    },
  ];
}

describe("pluginRegistry — review provider contracts", () => {
  afterEach(() => {
    pluginRegistry.unregisterPlugin(PRIMARY_PLUGIN_ID);
    pluginRegistry.unregisterPlugin(SECONDARY_PLUGIN_ID);
  });

  it("normalizes review items, cleans subscriptions, and aborts refresh on owner removal", async () => {
    const unsubscribe = vi.fn();
    const refreshAborted = vi.fn();
    let markRefreshStarted: () => void;
    const refreshStarted = new Promise<void>((resolve) => {
      markRefreshStarted = resolve;
    });
    const provider = reviewProvider(SOURCE_CONTROL_PROVIDER_ID, {
      getSnapshot: () => [
        {
          providerId: SOURCE_CONTROL_PROVIDER_ID,
          reviewKey: REVIEW_KEY,
          title: "Change 1",
          url: CHANGE_URL,
          connectionScope: CONNECTION_SCOPE,
          repositoryId: REPOSITORY_ID,
          changeRequestNumber: 1,
          state: "open",
          statusBadge: { label: "Checks passing", tone: "success" },
        },
        {
          providerId: "another-provider",
          reviewKey: "spoofed-change",
          title: "Spoofed",
          url: "https://example.test/changes/2",
          connectionScope: CONNECTION_SCOPE,
          repositoryId: REPOSITORY_ID,
          changeRequestNumber: 2,
          state: "open",
        },
      ],
      subscribe: () => unsubscribe,
      refresh: (_taskId, signal) =>
        new Promise((_, reject) => {
          markRefreshStarted();
          signal.addEventListener(
            "abort",
            () => {
              refreshAborted();
              reject(new Error("review refresh aborted"));
            },
            { once: true },
          );
        }),
    });
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerReviewProvider(provider);
    const registration = pluginRegistry.getReviewProvider(SOURCE_CONTROL_PROVIDER_ID);
    if (!registration) throw new Error("review provider registration missing");

    expect(registration.getSnapshot(TASK_ID)).toEqual([
      {
        providerId: SOURCE_CONTROL_PROVIDER_ID,
        reviewKey: REVIEW_KEY,
        title: "Change 1",
        url: CHANGE_URL,
        connectionScope: CONNECTION_SCOPE,
        repositoryId: REPOSITORY_ID,
        changeRequestNumber: 1,
        state: "open",
        statusBadge: { label: "Checks passing", tone: "success" },
      },
    ]);
    registration.subscribe(TASK_ID, () => {});
    const refresh = registration.refresh(TASK_ID, new AbortController().signal);
    await refreshStarted;

    pluginRegistry.unregisterPlugin(PRIMARY_PLUGIN_ID);

    await expect(refresh).rejects.toMatchObject({ name: "AbortError" });
    expect(unsubscribe).toHaveBeenCalledOnce();
    expect(refreshAborted).toHaveBeenCalledOnce();
    expect(pluginRegistry.getReviewProvider(SOURCE_CONTROL_PROVIDER_ID)).toBeUndefined();
  });

  it("preserves normalized task status and strips provider-specific fields", () => {
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerReviewProvider(
      reviewProvider(SOURCE_CONTROL_PROVIDER_ID, {
        getSnapshot: providerSpecificStatusSnapshot,
      }),
    );

    expect(
      pluginRegistry.getReviewProvider(SOURCE_CONTROL_PROVIDER_ID)?.getSnapshot(TASK_ID),
    ).toEqual(normalizedStatusSnapshot());
  });
});

describe("pluginRegistry — review provider associations", () => {
  afterEach(() => {
    pluginRegistry.unregisterPlugin(PRIMARY_PLUGIN_ID);
    pluginRegistry.unregisterPlugin(SECONDARY_PLUGIN_ID);
  });

  it("normalizes association snapshots and lifecycle-wraps association refresh and unlink", async () => {
    const refresh = vi.fn(async () => undefined);
    const unlink = vi.fn(async () => undefined);
    const subscribe = vi.fn(() => () => undefined);
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerReviewProvider(
      reviewProvider(SOURCE_CONTROL_PROVIDER_ID, {
        getAssociationSnapshot: () =>
          [
            {
              providerId: "spoofed",
              taskId: TASK_ID,
              reviewKey: REVIEW_KEY,
              connectionScope: CONNECTION_SCOPE,
              repositoryId: ` ${IMMUTABLE_REPOSITORY_ID} `,
              changeRequestNumber: " 1 ",
              providerPayload: "discard",
            },
            { providerId: "spoofed", taskId: "", reviewKey: "invalid" },
          ] as unknown as ReviewTaskAssociation[],
        subscribeAssociations: subscribe,
        refreshAssociations: refresh,
        unlink,
      }),
    );
    const registration = pluginRegistry.getReviewProvider(SOURCE_CONTROL_PROVIDER_ID)!;

    expect(registration.getAssociationSnapshot?.(WORKSPACE_ID)).toEqual([
      {
        providerId: SOURCE_CONTROL_PROVIDER_ID,
        taskId: TASK_ID,
        reviewKey: REVIEW_KEY,
        connectionScope: CONNECTION_SCOPE,
        repositoryId: IMMUTABLE_REPOSITORY_ID,
        changeRequestNumber: "1",
      },
    ]);
    registration.subscribeAssociations?.(WORKSPACE_ID, () => undefined);
    await registration.refreshAssociations?.(WORKSPACE_ID, new AbortController().signal);
    await registration.unlink?.({
      workspaceId: WORKSPACE_ID,
      taskId: TASK_ID,
      reviewKey: REVIEW_KEY,
      connectionScope: CONNECTION_SCOPE,
      repositoryId: IMMUTABLE_REPOSITORY_ID,
      changeRequestNumber: "1",
      signal: new AbortController().signal,
    });

    expect(subscribe).toHaveBeenCalledOnce();
    expect(refresh).toHaveBeenCalledWith(WORKSPACE_ID, expect.any(AbortSignal));
    expect(unlink).toHaveBeenCalledWith(
      expect.objectContaining({
        taskId: TASK_ID,
        repositoryId: IMMUTABLE_REPOSITORY_ID,
        changeRequestNumber: "1",
      }),
    );
  });
});

describe("pluginRegistry — review provider association identity", () => {
  afterEach(() => {
    pluginRegistry.unregisterPlugin(PRIMARY_PLUGIN_ID);
    pluginRegistry.unregisterPlugin(SECONDARY_PLUGIN_ID);
  });

  it("drops summaries and associations without immutable identity", () => {
    pluginRegistry.forPlugin(PRIMARY_PLUGIN_ID).registerReviewProvider(
      reviewProvider(SOURCE_CONTROL_PROVIDER_ID, {
        getSnapshot: () =>
          [
            {
              providerId: SOURCE_CONTROL_PROVIDER_ID,
              reviewKey: REVIEW_KEY,
              title: "Change 1",
              url: CHANGE_URL,
              connectionScope: CONNECTION_SCOPE,
              repositoryId: REPOSITORY_ID,
              state: "open",
            },
          ] as unknown as ReviewItemSummary[],
        getAssociationSnapshot: () =>
          [
            {
              providerId: SOURCE_CONTROL_PROVIDER_ID,
              taskId: TASK_ID,
              reviewKey: REVIEW_KEY,
            },
          ] as unknown as ReviewTaskAssociation[],
      }),
    );
    const registration = pluginRegistry.getReviewProvider(SOURCE_CONTROL_PROVIDER_ID)!;

    expect(registration.getSnapshot(TASK_ID)).toEqual([]);
    expect(registration.getAssociationSnapshot?.(WORKSPACE_ID)).toEqual([]);
  });

  it("rejects a review provider claimed by another active plugin", () => {
    pluginRegistry
      .forPlugin(PRIMARY_PLUGIN_ID)
      .registerReviewProvider(reviewProvider(SOURCE_CONTROL_PROVIDER_ID));

    expect(() =>
      pluginRegistry
        .forPlugin(SECONDARY_PLUGIN_ID)
        .registerReviewProvider(reviewProvider(SOURCE_CONTROL_PROVIDER_ID)),
    ).toThrow(
      `provider "${SOURCE_CONTROL_PROVIDER_ID}" is already owned by "${PRIMARY_PLUGIN_ID}"`,
    );
  });
});

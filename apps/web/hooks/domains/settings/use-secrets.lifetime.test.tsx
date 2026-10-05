import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { SecretListOptions } from "@/lib/api/domains/secrets-api";
import type { SecretListItem, SecretScope } from "@/lib/types/http-secrets";
import { useSecrets } from "./use-secrets";

const transport = vi.hoisted(() => ({ list: vi.fn() }));
vi.mock("@/lib/api/domains/secrets-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/secrets-api")>()),
  listSecrets: transport.list,
}));

const WORKSPACE_A = "workspace-a";
const WORKSPACE_B = "workspace-b";
const EMPTY: SecretListItem[] = [];
const SOURCES = ["supplied", "fetched"] as const;
const OUTCOMES = ["success", "failure"] as const;
const MUTATIONS = ["add", "update", "remove"] as const;
type Outcome = (typeof OUTCOMES)[number];
type Mutation = (typeof MUTATIONS)[number];

function metadata(id: string, workspaceId = WORKSPACE_A): SecretListItem {
  return {
    id,
    name: id,
    has_value: true,
    scope: "workspace",
    workspace_id: workspaceId,
    created_at: "2026-10-05T00:00:00Z",
    updated_at: "2026-10-05T00:00:00Z",
  };
}

const original = metadata("original");
const accepted = metadata("accepted");
const renamed = { ...original, name: "accepted rename" };
const globalItem: SecretListItem = {
  ...metadata("global"),
  scope: "global",
  workspace_id: undefined,
};

const pending: Array<() => void> = [];
function deferred() {
  let resolve!: (items: SecretListItem[]) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<SecretListItem[]>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  pending.push(() => resolve([]));
  return { promise, resolve, reject };
}

async function settle(
  read: ReturnType<typeof deferred>,
  items: SecretListItem[],
  outcome: Outcome = "success",
) {
  await act(async () => {
    if (outcome === "failure") read.reject(new Error("list unavailable"));
    else read.resolve(items);
  });
}

function wrapper({ children }: { children: ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}

type Props = {
  scope: SecretScope;
  workspaceId?: string;
  initialItems?: SecretListItem[];
  globalEnabled: boolean;
  secondGlobalEnabled: boolean;
};

function useSnapshot(props: Props) {
  return {
    workspace: useSecrets(props.scope, props.workspaceId, props.initialItems),
    global: useSecrets(props.globalEnabled ? "global" : "workspace", undefined, EMPTY),
    secondGlobal: useSecrets(props.secondGlobalEnabled ? "global" : "workspace", undefined, EMPTY),
    store: useAppStoreApi(),
  };
}

function renderSecrets(overrides: Partial<Props> = {}) {
  const props: Props = {
    scope: "workspace",
    workspaceId: WORKSPACE_A,
    globalEnabled: false,
    secondGlobalEnabled: false,
    ...overrides,
  };
  return { ...renderHook(useSnapshot, { wrapper, initialProps: props }), props };
}

function scopedCalls() {
  return transport.list.mock.calls
    .map(([options]) => options as SecretListOptions)
    .filter((options) => options.scope === "workspace");
}

function applyAcknowledgment(view: ReturnType<typeof useSecrets>, mutation: Mutation) {
  if (mutation === "add") view.addSecret(accepted);
  if (mutation === "update") view.updateSecret(renamed);
  if (mutation === "remove") view.removeSecret(original.id);
}

function expectedItems(mutation: Mutation) {
  if (mutation === "add") return [original, accepted];
  return mutation === "update" ? [renamed] : [];
}

beforeEach(() => {
  transport.list.mockReset();
});
afterEach(async () => {
  await act(async () => pending.splice(0).forEach((resolve) => resolve()));
  cleanup();
});

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.14
describe("independent workspace metadata lifetime", () => {
  const cases = SOURCES.flatMap((source) =>
    MUTATIONS.flatMap((mutation) => OUTCOMES.map((outcome) => ({ source, mutation, outcome }))),
  );

  it.each(cases)(
    "preserves acknowledged workspace mutations across global loading and settlement: $source $mutation $outcome",
    async ({ source, mutation, outcome }) => {
      const workspaceRead = deferred();
      const globalRead = deferred();
      transport.list.mockImplementation((options: SecretListOptions) =>
        options.scope === "workspace" ? workspaceRead.promise : globalRead.promise,
      );
      const initialItems = source === "supplied" ? [original] : undefined;
      const view = renderSecrets({ initialItems });
      if (source === "fetched") await settle(workspaceRead, [original]);

      act(() => applyAcknowledgment(view.result.current.workspace, mutation));
      const expected = { items: expectedItems(mutation), loaded: true, loading: false };
      // Ordinary mutation control, before admitting the unrelated Global consumer.
      expect(view.result.current.workspace).toMatchObject(expected);
      expect(scopedCalls()).toHaveLength(source === "fetched" ? 1 : 0);
      expect(view.result.current.store.getState().secrets.loaded).toBe(false);

      view.rerender({ ...view.props, globalEnabled: true });
      expect(view.result.current.global).toMatchObject({ loaded: false, loading: true });
      expect(view.result.current.workspace).toMatchObject(expected);
      await settle(globalRead, [globalItem], outcome);
      expect(view.result.current.global).toMatchObject({
        items: outcome === "success" ? [globalItem] : [],
        loaded: true,
        loading: false,
      });
      expect(view.result.current.store.getState().secrets).toMatchObject({
        loaded: true,
        loading: false,
      });
      expect(view.result.current.workspace).toMatchObject(expected);
      expect(scopedCalls()).toHaveLength(source === "fetched" ? 1 : 0);
    },
  );

  const pendingCases = OUTCOMES.flatMap((globalOutcome) =>
    OUTCOMES.map((workspaceOutcome) => ({ globalOutcome, workspaceOutcome })),
  );
  it.each(pendingCases)(
    "does not abort or refetch a pending workspace list during global loading: $globalOutcome/$workspaceOutcome",
    async ({ globalOutcome, workspaceOutcome }) => {
      const workspaceRead = deferred();
      const globalRead = deferred();
      transport.list.mockImplementation((options: SecretListOptions) =>
        options.scope === "workspace" ? workspaceRead.promise : globalRead.promise,
      );
      const view = renderSecrets();
      const signal = scopedCalls()[0].init?.signal;
      expect(signal?.aborted).toBe(false);
      const pendingView = { items: [], loaded: false, loading: true };
      expect(view.result.current.workspace).toMatchObject(pendingView);

      view.rerender({ ...view.props, globalEnabled: true });
      expect(view.result.current.global.loading).toBe(true);
      expect({ aborted: signal?.aborted, requests: scopedCalls().length }).toEqual({
        aborted: false,
        requests: 1,
      });
      expect(view.result.current.workspace).toMatchObject(pendingView);
      await settle(globalRead, [globalItem], globalOutcome);
      expect(view.result.current.global).toMatchObject({ loaded: true, loading: false });
      expect({ aborted: signal?.aborted, requests: scopedCalls().length }).toEqual({
        aborted: false,
        requests: 1,
      });
      expect(view.result.current.workspace).toMatchObject(pendingView);

      await settle(workspaceRead, [original], workspaceOutcome);
      expect(view.result.current.workspace).toMatchObject({
        items: workspaceOutcome === "success" ? [original] : [],
        loaded: true,
        loading: false,
      });
    },
  );
});

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.1
// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.2
// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.4
describe("global scope compatibility", () => {
  it("shares global loading and filters workspace rows", async () => {
    const read = deferred();
    transport.list.mockReturnValue(read.promise);
    const view = renderSecrets({
      workspaceId: undefined,
      initialItems: EMPTY,
      globalEnabled: true,
    });
    expect(view.result.current.global).toMatchObject({ loaded: false, loading: true });
    view.rerender({ ...view.props, secondGlobalEnabled: true });
    expect(view.result.current.secondGlobal).toMatchObject({ loaded: false, loading: true });
    expect(transport.list).toHaveBeenCalledExactlyOnceWith({ cache: "no-store" });

    const legacy: SecretListItem = {
      ...globalItem,
      id: "legacy",
      name: "legacy",
      scope: undefined,
    };
    await settle(read, [original, globalItem, legacy]);
    const expected = { items: [globalItem, legacy], loaded: true, loading: false };
    expect(view.result.current.global).toMatchObject(expected);
    expect(view.result.current.secondGlobal).toMatchObject(expected);
    expect(view.result.current.store.getState().secrets.items).toEqual([
      original,
      globalItem,
      legacy,
    ]);

    view.rerender({ ...view.props, globalEnabled: false, secondGlobalEnabled: true });
    view.rerender({ ...view.props, globalEnabled: true, secondGlobalEnabled: true });
    expect(view.result.current.global).toMatchObject(expected);
    expect(transport.list).toHaveBeenCalledTimes(1);
  });
});

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.2
describe("intentional scoped lifecycle changes", () => {
  it.each(["pending", "settled"] as const)(
    "replaces scoped data on workspace changes and rejects obsolete responses: %s",
    async (previousState) => {
      const first = deferred();
      const second = deferred();
      transport.list.mockImplementation((options: SecretListOptions) =>
        options.workspaceId === WORKSPACE_A ? first.promise : second.promise,
      );
      const view = renderSecrets();
      const signal = scopedCalls()[0].init?.signal;
      if (previousState === "settled") {
        await settle(first, [original]);
        expect(view.result.current.workspace.items).toEqual([original]);
      }
      view.rerender({ ...view.props, workspaceId: WORKSPACE_B });
      expect(view.result.current.workspace).toMatchObject({
        items: [],
        loaded: false,
        loading: true,
      });
      expect(signal?.aborted).toBe(true);
      const replacement = metadata("replacement", WORKSPACE_B);
      await settle(second, [replacement]);
      if (previousState === "pending") await settle(first, [original]);
      expect(view.result.current.workspace).toMatchObject({
        items: [replacement],
        loaded: true,
        loading: false,
      });
      expect(scopedCalls()).toHaveLength(2);
    },
  );

  it("replaces local mutations on a new initial-items reference and accepts an explicit empty list", () => {
    const view = renderSecrets({ initialItems: [original] });
    act(() => view.result.current.workspace.addSecret(accepted));
    expect(view.result.current.workspace.items).toEqual([original, accepted]);
    const replacement = metadata("replacement");
    view.rerender({ ...view.props, initialItems: [replacement] });
    expect(view.result.current.workspace).toMatchObject({
      items: [replacement],
      loaded: true,
      loading: false,
    });
    view.rerender({ ...view.props, initialItems: EMPTY });
    expect(view.result.current.workspace).toMatchObject({
      items: [],
      loaded: true,
      loading: false,
    });
    expect(transport.list).not.toHaveBeenCalled();
  });

  it("aborts a read when supplied initial items replace it and ignores late publication", async () => {
    const read = deferred();
    transport.list.mockReturnValue(read.promise);
    const view = renderSecrets({ initialItems: [original] });
    view.rerender({ ...view.props, initialItems: undefined });
    const signal = scopedCalls()[0].init?.signal;
    expect(view.result.current.workspace).toMatchObject({
      items: [],
      loaded: false,
      loading: true,
    });
    view.rerender({ ...view.props, initialItems: [accepted] });
    expect(signal?.aborted).toBe(true);
    await settle(read, [original]);
    expect(view.result.current.workspace).toMatchObject({
      items: [accepted],
      loaded: true,
      loading: false,
    });
    expect(scopedCalls()).toHaveLength(1);
  });
});

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.2
describe("scoped read cleanup", () => {
  it("cleans up scoped reads on scope changes without contaminating a later workspace", async () => {
    const workspaceRead = deferred();
    const globalRead = deferred();
    transport.list.mockImplementation((options: SecretListOptions) =>
      options.scope === "workspace" ? workspaceRead.promise : globalRead.promise,
    );
    const view = renderSecrets();
    const signal = scopedCalls()[0].init?.signal;
    view.rerender({ ...view.props, scope: "global" });
    expect(signal?.aborted).toBe(true);
    expect(view.result.current.workspace).toMatchObject({
      items: [],
      loaded: false,
      loading: true,
    });
    await settle(globalRead, [globalItem]);
    expect(view.result.current.workspace.items).toEqual([globalItem]);
    const replacement = metadata("replacement", WORKSPACE_B);
    view.rerender({ ...view.props, workspaceId: WORKSPACE_B, initialItems: [replacement] });
    await settle(workspaceRead, [original]);
    expect(view.result.current.workspace).toMatchObject({
      items: [replacement],
      loaded: true,
      loading: false,
    });
    expect(view.result.current.store.getState().secrets.items).toEqual([globalItem]);
  });

  it("does not fetch without a workspace ID", () => {
    const view = renderSecrets({ workspaceId: undefined });
    expect(view.result.current.workspace).toMatchObject({
      items: [],
      loaded: false,
      loading: false,
    });
    expect(transport.list).not.toHaveBeenCalled();
  });

  it("aborts a pending workspace read on unmount", async () => {
    const read = deferred();
    transport.list.mockReturnValue(read.promise);
    const view = renderSecrets();
    const signal = scopedCalls()[0].init?.signal;
    expect(signal?.aborted).toBe(false);
    view.unmount();
    expect(signal?.aborted).toBe(true);
    await settle(read, [original]);
    expect(view.result.current.store.getState().secrets).toMatchObject({
      items: [],
      loaded: false,
      loading: false,
    });
    expect(scopedCalls()).toHaveLength(1);
  });
});

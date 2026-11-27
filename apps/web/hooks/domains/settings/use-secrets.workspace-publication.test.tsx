import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { SecretListItem, SecretScope } from "@/lib/types/http-secrets";
import { useSecrets } from "./use-secrets";

const WORKSPACE = "publication-workspace";
const OTHER_WORKSPACE = "other-publication-workspace";
const EMPTY: SecretListItem[] = [];

function item(id: string, workspaceId = WORKSPACE): SecretListItem {
  return {
    id,
    name: `Metadata ${id}`,
    scope: "workspace",
    workspace_id: workspaceId,
    has_value: true,
    created_at: "2026-10-10T08:00:00Z",
    updated_at: "2026-10-10T08:00:00Z",
  };
}

function jsonResponse(items: SecretListItem[]) {
  return new Response(JSON.stringify(items), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function pendingRead(url: URL, init?: RequestInit) {
  let resolve!: (response: Response) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<Response>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { url, init, promise, resolve, reject };
}

const reads: ReturnType<typeof pendingRead>[] = [];
const fetchTransport = vi.fn();

beforeEach(() => {
  fetchTransport.mockReset();
  fetchTransport.mockImplementation((input: string, init?: RequestInit) => {
    const url = new URL(input, "http://localhost");
    expect(url.pathname).toBe("/api/v1/secrets");
    if (!url.searchParams.has("scope")) return Promise.resolve(jsonResponse([]));
    expect(url.searchParams.get("scope")).toBe("workspace");
    const read = pendingRead(url, init);
    reads.push(read);
    return read.promise;
  });
  vi.stubGlobal("fetch", fetchTransport);
});

afterEach(async () => {
  cleanup();
  await act(async () => {
    reads.splice(0).forEach((read) => read.resolve(jsonResponse([])));
  });
  vi.unstubAllGlobals();
});

function provider({ children }: { children: ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}

type Props = { scope: SecretScope; workspaceId?: string; initialItems?: SecretListItem[] };

function mount(overrides: Partial<Props> = {}) {
  const props: Props = { scope: "workspace", workspaceId: WORKSPACE, ...overrides };
  return {
    ...renderHook(
      (current: Props) => ({
        view: useSecrets(current.scope, current.workspaceId, current.initialItems),
        store: useAppStoreApi(),
      }),
      { initialProps: props, wrapper: provider },
    ),
    props,
  };
}

async function finish(read: ReturnType<typeof pendingRead>, items: SecretListItem[]) {
  await act(async () => {
    read.resolve(jsonResponse(items));
  });
}

// @covers AC-WORKSPACES-SECRET-CATALOGUE-001.2
// @covers AC-WORKSPACES-SECRET-CATALOGUE-001.3
describe("current workspace metadata during initial success", () => {
  it("preserves all mixed local changes against older metadata", async () => {
    const view = mount();
    expect(reads[0].url.searchParams.get("workspace_id")).toBe(WORKSPACE);
    expect(reads[0].init).toMatchObject({ cache: "no-store" });
    const first = item("first");
    const removed = item("removed");
    const last = item("last");
    const renamed = { ...first, name: "Acknowledged rename", updated_at: "2026-10-10T08:01:00Z" };
    act(() => {
      view.result.current.view.addSecret(first);
      view.result.current.view.addSecret(removed);
      view.result.current.view.addSecret(last);
      view.result.current.view.updateSecret(renamed);
      view.result.current.view.removeSecret(removed.id);
    });
    expect(view.result.current.view).toMatchObject({
      items: [renamed, last],
      loaded: false,
      loading: true,
    });
    view.rerender(view.props);
    expect(reads).toHaveLength(1);
    expect(reads[0].init?.signal?.aborted).toBe(false);
    await finish(reads[0], [first, removed, item("snapshot-only")]);
    expect(view.result.current.view).toMatchObject({
      items: [renamed, last],
      loaded: true,
      loading: false,
    });
    expect(reads).toHaveLength(1);
  });

  it("keeps an empty current list after add and removal", async () => {
    const view = mount();
    const accepted = item("accepted");
    act(() => {
      view.result.current.view.addSecret(accepted);
      view.result.current.view.removeSecret(accepted.id);
    });
    expect(view.result.current.view.items).toEqual([]);
    await finish(reads[0], [accepted, item("snapshot-only")]);
    expect(view.result.current.view).toMatchObject({ items: [], loaded: true, loading: false });
    expect(reads).toHaveLength(1);
  });

  it("keeps local authority after rename and restoration", async () => {
    const view = mount();
    const accepted = item("accepted");
    act(() => {
      view.result.current.view.addSecret(accepted);
      view.result.current.view.updateSecret({ ...accepted, name: "Temporary accepted rename" });
      view.result.current.view.updateSecret(accepted);
    });
    expect(view.result.current.view.items).toEqual([accepted]);
    await finish(reads[0], [accepted, item("snapshot-only")]);
    expect(view.result.current.view.items).toEqual([accepted]);
    expect(view.result.current.view).toMatchObject({ loaded: true, loading: false });
  });

  it.each([{ items: EMPTY }, { items: [item("ordinary"), item("second")] }])(
    "publishes an uncontested complete listing: $items",
    async ({ items }) => {
      const view = mount();
      expect(view.result.current.view).toMatchObject({ items: [], loaded: false, loading: true });
      await finish(reads[0], items);
      expect(view.result.current.view).toMatchObject({ items, loaded: true, loading: false });
      expect(reads).toHaveLength(1);
    },
  );

  it("retains ordinary initial-read failure settlement", async () => {
    const view = mount();
    await act(async () => {
      reads[0].reject(new Error("Synthetic initial-list transport failure"));
    });
    expect(view.result.current.view).toMatchObject({ items: [], loaded: true, loading: false });
    expect(reads).toHaveLength(1);
  });
});

// @covers AC-WORKSPACES-SECRET-CATALOGUE-001.4
// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.2
describe("workspace publication lifetime controls", () => {
  it("accepts a new workspace after canceling the old read", async () => {
    const view = mount();
    act(() => {
      view.result.current.view.addSecret(item("accepted-in-first"));
    });
    view.rerender({ ...view.props, workspaceId: OTHER_WORKSPACE });
    expect(reads[0].init?.signal?.aborted).toBe(true);
    expect(reads[1].url.searchParams.get("workspace_id")).toBe(OTHER_WORKSPACE);
    await finish(reads[0], [item("late-first")]);
    expect(view.result.current.view).toMatchObject({ items: [], loaded: false, loading: true });
    const replacement = item("second-workspace", OTHER_WORKSPACE);
    await finish(reads[1], [replacement]);
    expect(view.result.current.view).toMatchObject({
      items: [replacement],
      loaded: true,
      loading: false,
    });
    expect(reads).toHaveLength(2);
  });

  it("replaces mutations with supplied initial items including empty", async () => {
    const view = mount();
    act(() => {
      view.result.current.view.addSecret(item("accepted"));
    });
    const supplied = [item("supplied")];
    view.rerender({ ...view.props, initialItems: supplied });
    expect(reads[0].init?.signal?.aborted).toBe(true);
    await finish(reads[0], [item("obsolete")]);
    expect(view.result.current.view).toMatchObject({
      items: supplied,
      loaded: true,
      loading: false,
    });
    act(() => {
      view.result.current.view.addSecret(item("another-accepted"));
    });
    view.rerender({ ...view.props, initialItems: EMPTY });
    expect(view.result.current.view).toMatchObject({ items: [], loaded: true, loading: false });
    expect(reads).toHaveLength(1);
    view.rerender(view.props);
    await finish(reads[1], [item("fresh-uncontested")]);
    expect(view.result.current.view.items).toEqual([item("fresh-uncontested")]);
  });

  it("cancels on scope change without touching global metadata", async () => {
    const view = mount();
    act(() => {
      view.result.current.view.addSecret(item("workspace-only"));
    });
    await act(async () => {
      view.rerender({ ...view.props, scope: "global" });
    });
    expect(reads[0].init?.signal?.aborted).toBe(true);
    await finish(reads[0], [item("obsolete")]);
    expect(view.result.current.view).toMatchObject({ items: [], loaded: true, loading: false });
    expect(view.result.current.store.getState().secrets.items).toEqual([]);
  });

  it("abandons a mutated pending listing on unmount", async () => {
    const view = mount();
    act(() => {
      view.result.current.view.addSecret(item("accepted"));
    });
    const store = view.result.current.store;
    view.unmount();
    expect(reads[0].init?.signal?.aborted).toBe(true);
    await finish(reads[0], [item("obsolete")]);
    expect(store.getState().secrets).toMatchObject({ items: [], loaded: false, loading: false });
    expect(reads).toHaveLength(1);
  });

  it("does not fetch without a workspace ID or with supplied empty items", () => {
    const view = mount({ workspaceId: undefined });
    expect(view.result.current.view).toMatchObject({ items: [], loaded: false, loading: false });
    view.rerender({ ...view.props, workspaceId: WORKSPACE, initialItems: EMPTY });
    expect(view.result.current.view).toMatchObject({ items: [], loaded: true, loading: false });
    expect(fetchTransport).not.toHaveBeenCalled();
  });
});

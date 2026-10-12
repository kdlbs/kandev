import type { PropsWithChildren } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { SecretListItem, SecretScope } from "@/lib/types/http-secrets";
import { useSecretDestinationNames } from "./use-secret-destination-names";

type Ticket = {
  url: string;
  init?: RequestInit;
  promise: Promise<Response>;
  resolve: (response: Response) => void;
  reject: (error: Error) => void;
  settled: boolean;
};
let tickets: Ticket[];
const timestamp = "2026-10-06T00:00:00Z";

function secret(name: string, workspaceId = "alpha"): SecretListItem {
  return {
    id: `secret-${name}`,
    name,
    scope: "workspace",
    workspace_id: workspaceId,
    has_value: true,
    created_at: timestamp,
    updated_at: timestamp,
  };
}

function Wrapper({ children }: PropsWithChildren) {
  return <StateProvider>{children}</StateProvider>;
}

beforeEach(() => {
  tickets = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      let resolve!: Ticket["resolve"];
      let reject!: Ticket["reject"];
      const promise = new Promise<Response>((yes, no) => {
        resolve = yes;
        reject = no;
      });
      tickets.push({ url: String(input), init, promise, resolve, reject, settled: false });
      return promise;
    }),
  );
});

async function respond(index: number, names: string[] = [], status = 200) {
  const ticket = tickets[index];
  expect(ticket).toBeDefined();
  await act(async () => {
    ticket.settled = true;
    ticket.resolve(new Response(JSON.stringify(names.map((name) => secret(name))), { status }));
    await ticket.promise;
  });
}

async function reject(index: number) {
  const ticket = tickets[index];
  await act(async () => {
    ticket.settled = true;
    ticket.reject(new Error("Transport unavailable"));
    await ticket.promise.catch(() => undefined);
  });
}

afterEach(async () => {
  cleanup();
  await act(async () => {
    for (const ticket of tickets.filter((item) => !item.settled)) {
      ticket.settled = true;
      ticket.resolve(new Response("[]", { status: 200 }));
    }
    await Promise.allSettled(tickets.map((ticket) => ticket.promise));
  });
  vi.unstubAllGlobals();
});

type DestinationProps = { scope: SecretScope; workspaceId?: string; refreshKey: number };

function renderDestination() {
  return renderHook<ReturnType<typeof useSecretDestinationNames>, DestinationProps>(
    ({
      scope,
      workspaceId,
      refreshKey,
    }: {
      scope: SecretScope;
      workspaceId?: string;
      refreshKey: number;
    }) => useSecretDestinationNames(scope, workspaceId, refreshKey),
    {
      initialProps: { scope: "workspace" as SecretScope, workspaceId: "alpha", refreshKey: 0 },
      wrapper: Wrapper,
    },
  );
}

const alpha = { scope: "workspace" as SecretScope, workspaceId: "alpha", refreshKey: 0 };
const global = { scope: "global" as SecretScope, workspaceId: undefined, refreshKey: 0 };

describe("cancelled destination lookup recovery", () => {
  // @covers AC-WORKSPACES-SECRET-SCOPE-TRANSFER-001.9
  it("replaces a pending lookup after a scope roundtrip", async () => {
    const view = renderDestination();
    expect(view.result.current.loaded).toBe(false);
    view.rerender(global);
    view.rerender(alpha);
    expect(tickets).toHaveLength(2);
    expect(tickets[1].url).toContain("scope=workspace&workspace_id=alpha");
    expect(tickets[1].init?.cache).toBe("no-store");
    await respond(1, ["Current"]);
    await respond(0, ["Abandoned"]);
    expect(view.result.current.loaded).toBe(true);
    expect(view.result.current.names).toEqual(["Current"]);
    expect(view.result.current.conflict(" Current ")).toBe(true);
    expect(view.result.current.conflict("Abandoned")).toBe(false);
  });

  it("restarts the standalone StrictMode setup", async () => {
    const view = renderHook(() => useSecretDestinationNames("workspace", "alpha"), {
      wrapper: Wrapper,
      reactStrictMode: true,
    });
    expect(tickets).toHaveLength(2);
    await respond(0, ["Abandoned"]);
    expect(view.result.current.loaded).toBe(false);
    await respond(1, ["Current"]);
    expect(view.result.current.loaded).toBe(true);
    expect(view.result.current.names).toEqual(["Current"]);
  });

  it.each(["success", "failure"] as const)(
    "ignores abandoned %s before the replacement settles",
    async (outcome) => {
      const view = renderDestination();
      view.rerender(global);
      view.rerender(alpha);
      expect(tickets).toHaveLength(2);
      if (outcome === "success") await respond(0, ["Abandoned"]);
      else await reject(0);
      expect(view.result.current.loaded).toBe(false);
      expect(view.result.current.conflict("Abandoned")).toBe(false);
      await respond(1, ["Current"]);
      expect(view.result.current.names).toEqual(["Current"]);
      expect(view.result.current.loaded).toBe(true);
    },
  );

  // @covers AC-WORKSPACES-SECRET-SCOPE-TRANSFER-001.10
  it("reuses a completed entry until another workspace read invalidates it", async () => {
    const view = renderDestination();
    await respond(0, ["Alpha"]);
    view.rerender(global);
    view.rerender(alpha);
    expect(tickets).toHaveLength(1);
    expect(view.result.current.conflict("Alpha")).toBe(true);
    view.rerender({ ...alpha, workspaceId: "beta" });
    view.rerender(alpha);
    expect(tickets).toHaveLength(3);
    await respond(1, ["Abandoned beta"]);
    expect(view.result.current.loaded).toBe(false);
    await respond(2, ["Fresh alpha"]);
    expect(view.result.current.names).toEqual(["Fresh alpha"]);
  });
});

describe("destination lookup preservation", () => {
  it("refreshes a completed same-workspace entry explicitly", async () => {
    const view = renderDestination();
    await respond(0, ["Old"]);
    view.rerender({ ...alpha, refreshKey: 1 });
    expect(tickets).toHaveLength(2);
    expect(view.result.current.loaded).toBe(false);
    await respond(1, ["Current"]);
    expect(view.result.current.names).toEqual(["Current"]);
    view.rerender({ ...alpha, refreshKey: 1 });
    expect(tickets).toHaveLength(2);
  });

  it("ignores a pending earlier refresh after the current refresh settles", async () => {
    const view = renderDestination();
    view.rerender({ ...alpha, refreshKey: 1 });
    await respond(1, ["Current"]);
    await reject(0);
    expect(view.result.current.loaded).toBe(true);
    expect(view.result.current.names).toEqual(["Current"]);
  });

  it("isolates changed-workspace publication", async () => {
    const view = renderDestination();
    view.rerender({ ...alpha, workspaceId: "beta" });
    await respond(1, ["Beta"]);
    await respond(0, ["Abandoned alpha"]);
    expect(tickets[1].url).toContain("workspace_id=beta");
    expect(view.result.current.names).toEqual(["Beta"]);
    expect(view.result.current.conflict("Abandoned alpha")).toBe(false);
  });

  // @covers AC-WORKSPACES-SECRET-SCOPE-TRANSFER-001.11
  it.each(["transport", "http"] as const)(
    "keeps the completed %s failure fallback reusable",
    async (failure) => {
      const view = renderDestination();
      if (failure === "transport") await reject(0);
      else await respond(0, [], 500);
      expect(view.result.current.loaded).toBe(true);
      expect(view.result.current.names).toEqual([]);
      expect(view.result.current.conflict("Anything")).toBe(false);
      view.rerender(global);
      view.rerender(alpha);
      expect(tickets).toHaveLength(1);
      expect(view.result.current.loaded).toBe(true);
    },
  );

  it("reads reactive Global names from the real store without workspace entries", () => {
    const view = renderHook(
      () => ({ destination: useSecretDestinationNames("global"), store: useAppStoreApi() }),
      { wrapper: Wrapper },
    );
    act(() =>
      view.result.current.store
        .getState()
        .setSecrets([
          { ...secret("Global"), scope: "global", workspace_id: undefined },
          secret("Workspace"),
        ]),
    );
    expect(view.result.current.destination.names).toEqual(["Global"]);
    expect(view.result.current.destination.loaded).toBe(true);
    expect(view.result.current.destination.conflict("Workspace")).toBe(false);
    act(() =>
      view.result.current.store
        .getState()
        .setSecrets([{ ...secret("Renamed"), scope: "global", workspace_id: undefined }]),
    );
    expect(view.result.current.destination.names).toEqual(["Renamed"]);
    expect(tickets).toHaveLength(0);
  });
});

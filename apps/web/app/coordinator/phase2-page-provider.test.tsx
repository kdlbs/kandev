import { cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { StandingOrder } from "@/lib/api/domains/coordinator-api";

const ordersState = vi.hoisted(() => ({
  value: { orders: [] as unknown[], status: "loading", reload: vi.fn() },
}));
const pruneMock = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/domains/coordinator/use-standing-orders", () => ({
  useStandingOrders: () => ordersState.value,
}));
vi.mock("./use-stall-resume", () => ({
  pruneStallResumes: (...args: unknown[]) => pruneMock(...args),
}));
vi.mock("./reject-offer-host", () => ({
  RejectOfferHost: () => null,
  useRejectOffer: () => ({ offer: null, offerReject: vi.fn(), close: vi.fn() }),
}));

import { Phase2PageProvider } from "./phase2-page-provider";
import { usePhase2CardContext, type Phase2CardContextValue } from "./proposal-card/phase2-context";

let seen: Phase2CardContextValue | undefined;
function Probe() {
  seen = usePhase2CardContext();
  return null;
}

function mount(proposalsKey = "a", stallIds: string[] = [], loaded = true) {
  const tree = (key: string, ids: string[], inputsLoaded: boolean) => (
    <Phase2PageProvider
      workspaceId="w"
      coordinatorId="c"
      proposalsKey={key}
      liveStallTaskIds={ids}
      inputsLoaded={inputsLoaded}
    >
      <Probe />
    </Phase2PageProvider>
  );
  const view = render(tree(proposalsKey, stallIds, loaded));
  return (key: string, ids: string[], inputsLoaded = true) =>
    view.rerender(tree(key, ids, inputsLoaded));
}

beforeEach(() => {
  seen = undefined;
  pruneMock.mockReset();
  ordersState.value = { orders: [{ id: "o-1" }], status: "loading", reload: vi.fn() };
});
afterEach(cleanup);

describe("Phase2PageProvider", () => {
  it("exposes orders only once the read is ready, so a failed read shows no labels", () => {
    mount();
    expect(seen?.enabled).toBe(true);
    expect(seen?.orders).toBeUndefined();

    cleanup();
    ordersState.value = { ...ordersState.value, status: "error" };
    mount();
    expect(seen?.orders).toBeUndefined();

    cleanup();
    ordersState.value = { ...ordersState.value, status: "ready" };
    mount();
    expect(seen?.orders as StandingOrder[] | undefined).toEqual([{ id: "o-1" }]);
    expect(seen?.offerReject).toBeTypeOf("function");
  });

  it("reloads the orders when the proposals key changes, not on first render", () => {
    const rerender = mount("a");
    expect(ordersState.value.reload).not.toHaveBeenCalled();
    rerender("a", []);
    expect(ordersState.value.reload).not.toHaveBeenCalled();
    rerender("b", []);
    expect(ordersState.value.reload).toHaveBeenCalledTimes(1);
  });

  it("does not prune held Resumes while the page inputs are still loading", () => {
    const rerender = mount("a", [], false);
    expect(pruneMock).not.toHaveBeenCalled();
    rerender("a", [], false);
    expect(pruneMock).not.toHaveBeenCalled();
    rerender("a", ["t-1"], true);
    expect(pruneMock).toHaveBeenLastCalledWith(new Set(["t-1"]));
  });

  it("prunes held Resumes to the stalls now on the page", () => {
    const rerender = mount("a", ["t-1", "t-2"]);
    expect(pruneMock).toHaveBeenLastCalledWith(new Set(["t-1", "t-2"]));
    rerender("a", []);
    expect(pruneMock).toHaveBeenLastCalledWith(new Set());
  });
});

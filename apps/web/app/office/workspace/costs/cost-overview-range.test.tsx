import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import { t } from "@/lib/i18n";
import { CostOverview } from "./cost-overview";

vi.mock("@/lib/config", () => ({ getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }) }));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const MTD_TOTAL = "$1.00";
const CURRENT_USAGE_LABEL = "Current usage";
const PREVIOUS_USAGE_LABEL = "Previous usage";

function breakdown(range: string | null) {
  const groups = [
    { group_key: "current", group_label: CURRENT_USAGE_LABEL, count: 3, total_subcents: 10000 },
  ];
  if (range !== "mtd")
    groups.push({
      group_key: "previous",
      group_label: PREVIOUS_USAGE_LABEL,
      count: 7,
      total_subcents: 30000,
    });
  if (range === null)
    groups.push({ group_key: "old", group_label: "Old usage", count: 5, total_subcents: 20000 });
  return {
    total_subcents: groups.reduce((sum, g) => sum + g.total_subcents, 0),
    by_agent: groups,
    by_project: groups,
    by_model: groups,
    by_provider: groups,
  };
}

function CaptureStore({ store }: { store: { current: StoreApi<AppState> | null } }) {
  store.current = useAppStoreApi();
  return null;
}

// @covers AC-OFFICE-COSTS-001.8: actual buttons -> real API -> fetch -> displayed data.
describe("Costs selected range transport", () => {
  it("displays MTD and rolling 30-day groups and keeps the selected range on live refresh", async () => {
    const urls: URL[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = new URL(String(input));
        urls.push(url);
        return new Response(JSON.stringify(breakdown(url.searchParams.get("range"))), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    const store: { current: StoreApi<AppState> | null } = { current: null };
    render(
      <StateProvider initialState={{ workspaces: { activeId: "ws-range", items: [] } }}>
        <CaptureStore store={store} />
        <CostOverview workspaceId="ws-range" />
      </StateProvider>,
    );
    await waitFor(() => expect(screen.getAllByText(MTD_TOTAL)).toHaveLength(5));
    expect(urls[0].pathname).toBe("/api/v1/office/workspaces/ws-range/costs/breakdown");
    expect(urls[0].searchParams.get("range")).toBe("mtd");
    expect(screen.queryByText("Old usage")).toBeNull();
    expect(screen.queryByText(PREVIOUS_USAGE_LABEL)).toBeNull();
    assertGroupCards("1");
    assertEventCounts(CURRENT_USAGE_LABEL, "3");

    fireEvent.click(screen.getByRole("button", { name: t("office:last30Days") }));
    await waitFor(() => expect(screen.getByText("$4.00")).toBeTruthy());
    expect(urls.at(-1)?.searchParams.get("range")).toBe("30d");
    expect(screen.getAllByText(PREVIOUS_USAGE_LABEL)).toHaveLength(4);
    assertGroupCards("2");
    assertEventCounts(PREVIOUS_USAGE_LABEL, "7");
    act(() => store.current!.getState().setOfficeRefetchTrigger("costs"));
    await waitFor(() => expect(urls).toHaveLength(3));
    expect(urls[2].searchParams.get("range")).toBe("30d");

    fireEvent.click(screen.getByRole("button", { name: "MTD" }));
    await waitFor(() => expect(screen.queryByText("$4.00")).toBeNull());
    expect(urls.at(-1)?.searchParams.get("range")).toBe("mtd");
    expect(screen.queryByText(PREVIOUS_USAGE_LABEL)).toBeNull();
  });

  it("does not label prior-window spending as a pending or failed range", async () => {
    let rejectRange!: (error: Error) => void;
    const urls: URL[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = new URL(String(input));
        urls.push(url);
        if (url.searchParams.get("range") === "30d") {
          return new Promise<Response>((_resolve, reject) => {
            rejectRange = reject;
          });
        }
        return Promise.resolve(
          new Response(JSON.stringify(breakdown("mtd")), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
        );
      }),
    );
    render(
      <StateProvider initialState={{ workspaces: { activeId: "ws-range", items: [] } }}>
        <CostOverview workspaceId="ws-range" />
      </StateProvider>,
    );
    await waitFor(() => expect(screen.getAllByText(MTD_TOTAL)).toHaveLength(5));
    fireEvent.click(screen.getByRole("button", { name: t("office:last30Days") }));
    await waitFor(() => expect(urls).toHaveLength(2));
    try {
      expect(screen.queryAllByText(CURRENT_USAGE_LABEL)).toHaveLength(0);
      expect(screen.queryAllByText(MTD_TOTAL)).toHaveLength(0);
      assertGroupCards("0");
    } finally {
      await act(async () => rejectRange(new Error("selected range unavailable")));
    }
    expect(screen.queryAllByText(CURRENT_USAGE_LABEL)).toHaveLength(0);
    expect(screen.queryAllByText(MTD_TOTAL)).toHaveLength(0);
    assertGroupCards("0");
    expect(
      screen.getByRole("button", { name: t("office:last30Days") }).getAttribute("data-variant"),
    ).toBe("secondary");
  });
});

function assertGroupCards(count: string) {
  for (const key of ["activeAgents", "projects", "modelsUsed", "providers"]) {
    expect(
      within(screen.getByText(t(`office:${key}`)).parentElement!).getByText(count),
    ).toBeTruthy();
  }
}
function assertEventCounts(label: string, count: string) {
  for (const item of screen.getAllByText(label))
    expect(within(item.parentElement!).getByText(count)).toBeTruthy();
}

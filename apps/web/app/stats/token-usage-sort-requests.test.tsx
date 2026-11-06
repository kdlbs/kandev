import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { ApiRequestOptions } from "@/lib/api/client";
import type { TokenUsageResponse } from "@/lib/types/http";
import { retainedReport } from "./token-usage.test-helpers";

const http = vi.hoisted(() => ({
  fetchJson: vi.fn<(path: string, options?: ApiRequestOptions) => Promise<TokenUsageResponse>>(),
}));

vi.mock("@/lib/api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/client")>()),
  fetchJson: http.fetchJson,
}));
vi.mock("@/hooks/domains/plugins/use-plugins", () => ({ usePlugins: () => ({ items: [] }) }));
vi.mock("@/hooks/use-copy-to-clipboard", () => ({
  useCopyToClipboard: () => ({ copy: vi.fn(), copied: false }),
}));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ replace: vi.fn() }),
  usePathname: () => "/stats/token-usage",
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("@/components/page-shell", () => ({
  PageShell: ({ actions, children }: { actions: ReactNode; children: ReactNode }) => (
    <section>
      {actions}
      {children}
    </section>
  ),
}));

import { TokenUsagePageClient } from "./token-usage-page-client";

const TABLE_ID = "token-usage-table";
const DEFAULT_SORT = "cost_subcents";
const RATE_LABEL = "Effective cost / 1M tokens";
const RATE_KEY = "effective_cost_per_million";
const DIRECTIONS = ["asc", "desc"] as const;
const LABEL_SORTS = [
  { label: "Models", group: "model", sort: "model", control: "label", initial: DEFAULT_SORT },
  { label: "Tasks", group: "task", sort: "task", control: "label", initial: DEFAULT_SORT },
  { label: "Sessions", group: "session", sort: "session", control: "label", initial: DEFAULT_SORT },
  { label: "Daily", group: "day", sort: "period", control: "period", initial: "period" },
  { label: "Monthly", group: "month", sort: "period", control: "period", initial: "period" },
] as const;

function latestBreakdownQuery() {
  const requests = http.fetchJson.mock.calls.map(([path]) => new URL(path, "http://usage.test"));
  const request = requests.findLast((url) => url.searchParams.get("limit") === "200");
  expect(request?.pathname).toBe("/api/v1/workspaces/a/stats/token-usage");
  return Object.fromEntries(request!.searchParams);
}

async function expectRequest(sort: string, direction: string, offset = 0, group = "model") {
  await waitFor(() => {
    expect(latestBreakdownQuery()).toEqual({
      range: "month",
      group,
      include_undated: "false",
      timezone: expect.any(String),
      sort,
      direction,
      limit: "200",
      offset: String(offset),
    });
  });
  await screen.findByTestId(TABLE_ID);
}

async function nextPage(sort: string, direction: string, group = "model") {
  fireEvent.click(screen.getByRole("button", { name: "Next page" }));
  await expectRequest(sort, direction, 200, group);
}

function header(label: string) {
  return within(screen.getByTestId(TABLE_ID)).getByRole("button", { name: label });
}

async function selectView(view: (typeof LABEL_SORTS)[number]) {
  const breakdown = screen.getByTestId("token-usage-breakdown");
  fireEvent.mouseDown(within(breakdown).getByRole("tab", { name: view.label }), {
    button: 0,
    ctrlKey: false,
  });
  await expectRequest(view.initial, "desc", 0, view.group);
}

beforeEach(async () => {
  http.fetchJson.mockReset();
  http.fetchJson.mockImplementation(async (path) => {
    const query = new URL(path, "http://usage.test").searchParams;
    return retainedReport(query.get("limit") === "200" ? { total_rows: 201, has_more: true } : {});
  });
  render(<TokenUsagePageClient workspaceId="a" />);
  await expectRequest(DEFAULT_SORT, "desc");
});
afterEach(cleanup);

describe("desktop usage sort requests", () => {
  it.each([
    ["Input", "input"],
    ["Output", "output"],
    ["Cache read", "cache_read"],
    ["Cache write", "cache_write"],
    ["Reasoning", "reasoning"],
    ["Total tokens", "total_tokens"],
    ["Cost", DEFAULT_SORT],
    [RATE_LABEL, "rate"],
  ])("uses the server key for %s and toggles direction from page one", async (label, sort) => {
    await nextPage(DEFAULT_SORT, "desc");
    fireEvent.click(header(label));
    const firstDirection = sort === DEFAULT_SORT ? "asc" : "desc";
    await expectRequest(sort, firstDirection);
    expect(header(label).closest("th")?.getAttribute("aria-sort")).toBe(`${firstDirection}ending`);

    await nextPage(sort, firstDirection);
    fireEvent.click(header(label));
    const nextDirection = firstDirection === "asc" ? "desc" : "asc";
    await expectRequest(sort, nextDirection);
    expect(header(label).closest("th")?.getAttribute("aria-sort")).toBe(`${nextDirection}ending`);
  });

  it.each(LABEL_SORTS)("preserves the $label label key and page reset", async (view) => {
    await selectView(view);
    await nextPage(view.initial, "desc", view.group);
    fireEvent.click(header(view.label));
    await expectRequest(view.sort, "asc", 0, view.group);
    await nextPage(view.sort, "asc", view.group);
    fireEvent.click(header(view.label));
    await expectRequest(view.sort, "desc", 0, view.group);
  });
});

describe("phone usage sort requests", () => {
  it.each([
    ["total_tokens", "total_tokens"],
    [DEFAULT_SORT, DEFAULT_SORT],
    [RATE_KEY, "rate"],
  ])("preserves the %s key and explicit directions with page reset", async (control, sort) => {
    let previousSort = DEFAULT_SORT;
    let previousDirection: string = "desc";
    for (const direction of DIRECTIONS) {
      await nextPage(previousSort, previousDirection);
      fireEvent.change(screen.getByLabelText("Sort"), {
        target: { value: `${control}:${direction}` },
      });
      await expectRequest(sort, direction);
      expect((screen.getByLabelText("Sort") as HTMLSelectElement).value).toBe(
        `${control}:${direction}`,
      );
      previousSort = sort;
      previousDirection = direction;
    }
  });

  it.each(LABEL_SORTS)("preserves the $label phone key and explicit directions", async (view) => {
    await selectView(view);
    let previousSort: string = view.initial;
    let previousDirection: string = "desc";
    for (const direction of DIRECTIONS) {
      await nextPage(previousSort, previousDirection, view.group);
      fireEvent.change(screen.getByLabelText("Sort"), {
        target: { value: `${view.control}:${direction}` },
      });
      await expectRequest(view.sort, direction, 0, view.group);
      previousSort = view.sort;
      previousDirection = direction;
    }
  });
});

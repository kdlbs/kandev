import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { retainedReport, retainedRow } from "./token-usage.test-helpers";
import type { TokenUsageLoadState } from "./token-usage-data";

const COPY_STATS_LABEL = "Copy Stats";

const mocks = vi.hoisted(() => ({
  state: null as TokenUsageLoadState | null,
  copy: vi.fn(),
  replace: vi.fn(),
}));
vi.mock("./token-usage-data", async () => ({
  ...(await vi.importActual<typeof import("./token-usage-data")>("./token-usage-data")),
  useTokenUsage: () => mocks.state,
}));
vi.mock("@/hooks/domains/plugins/use-plugins", () => ({ usePlugins: () => ({ items: [] }) }));
vi.mock("@/hooks/use-copy-to-clipboard", () => ({
  useCopyToClipboard: () => ({ copy: mocks.copy, copied: false }),
}));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ replace: mocks.replace }),
  usePathname: () => "/stats/token-usage",
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("@/components/page-shell", () => ({
  PageShell: ({
    title,
    actions,
    children,
  }: {
    title: string;
    actions: ReactNode;
    children: ReactNode;
  }) => (
    <section>
      <h1>{title}</h1>
      {actions}
      {children}
    </section>
  ),
}));
import { TokenUsagePageClient } from "./token-usage-page-client";

describe("retained usage page", () => {
  afterEach(cleanup);
  beforeEach(() => {
    mocks.copy.mockReset();
    mocks.state = { status: "ready", key: "query", data: retainedReport() };
  });

  it("shows first projection preparation instead of empty authoritative usage", () => {
    mocks.state = {
      status: "ready",
      key: "query",
      data: retainedReport({
        projection_status: "preparing",
        projection_generation: "",
        projection_pending: true,
        has_history: false,
        rows: [],
      }),
    };
    render(<TokenUsagePageClient workspaceId="a" />);
    expect(screen.getByRole("status").textContent).toContain("Preparing usage history.");
    expect(screen.getByRole("button", { name: "Retry status" })).toBeTruthy();
    expect(screen.queryByTestId("token-usage-empty-history")).toBeNull();
    expect(screen.queryByTestId("token-usage-summary")).toBeNull();
    expect(screen.getByRole("button", { name: COPY_STATS_LABEL }).hasAttribute("disabled")).toBe(
      true,
    );
  });

  it("keeps saved amounts, discloses pending/gaps, and copies complete filtered totals", () => {
    const total = retainedRow({
      total_tokens: null,
      amounts_decimal: { total_tokens: "9007199254740993" },
      cost_basis: "reported",
    });
    mocks.state = {
      status: "ready",
      key: "query",
      error: "temporarily busy",
      data: retainedReport({
        summary: retainedRow({ total_tokens: 777 }),
        totals: total,
        projection_status: "stale",
        projection_pending: true,
        history_gap: true,
      }),
    };
    render(<TokenUsagePageClient workspaceId="a" />);
    const summary = screen.getByTestId("token-usage-summary");
    expect(within(summary).getByText("9,007,199,254,740,993")).toBeTruthy();
    expect(within(summary).queryByText("777")).toBeNull();
    expect(
      screen.getAllByText("Saved values remain visible. Updating history...").length,
    ).toBeGreaterThan(0);
    expect(screen.getByText("Some history from before migration is unavailable.")).toBeTruthy();
    expect(screen.getAllByText("temporarily busy").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: COPY_STATS_LABEL }));
    const text = mocks.copy.mock.calls[0][0] as string;
    expect(text).toContain("Total tokens: 9,007,199,254,740,993");
    expect(text).toContain("Cost basis: Reported cost");
    expect(text).toContain("Provider: All providers");
    expect(text).toContain("Calendar: Asia/Kathmandu");
    expect(text).toContain("Cost user attribution: Unknown");
    expect(text).toContain("Effective cost / 1M tokens: Unavailable");
    expect(text).not.toContain("777");
  });

  it("keeps projection freshness visible for an empty saved date range", () => {
    mocks.state = {
      status: "ready",
      key: "query",
      data: retainedReport({ has_range_data: false, rows: [], projection_pending: true }),
    };
    render(<TokenUsagePageClient workspaceId="a" />);
    expect(screen.getByRole("status").textContent).toContain(
      "Saved values remain visible. Updating history...",
    );
    expect(screen.getByTestId("token-usage-empty-range")).toBeTruthy();
    expect(screen.queryByTestId("token-usage-empty-history")).toBeNull();
  });
});

describe("retained monetary totals", () => {
  afterEach(cleanup);
  it("localizes mixed currencies and never copies one combined monetary amount", () => {
    mocks.copy.mockReset();
    const totals = retainedRow({
      currency: "mixed",
      cost_subcents: null,
      cost_coverage: "partial",
    });
    mocks.state = { status: "ready", key: "query", data: retainedReport({ totals }) };
    render(<TokenUsagePageClient workspaceId="a" />);
    expect(screen.getByText("Multiple currencies. Monetary amounts remain separate.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: COPY_STATS_LABEL }));
    const text = mocks.copy.mock.calls[0][0] as string;
    expect(text).toContain("Cost: Unavailable");
    expect(text).toContain("Currency: Multiple currencies. Monetary amounts remain separate.");
    expect(text).not.toContain("Currency: mixed");
  });
});

describe("unallocated usage copy", () => {
  afterEach(cleanup);
  it.each(["Tasks", "Sessions"])(
    "copies the unallocated label and complete filtered amounts for %s",
    (view) => {
      mocks.copy.mockReset();
      mocks.state = {
        status: "ready",
        key: "query",
        data: retainedReport({
          totals: retainedRow({ total_tokens: 4_000, cost_subcents: 20_000 }),
          rows: [retainedRow({ task_id: "", session_id: "", total_tokens: 100 })],
          total_rows: 300,
          has_more: true,
        }),
      };
      render(<TokenUsagePageClient workspaceId="a" />);
      const breakdown = screen.getByTestId("token-usage-breakdown");
      fireEvent.mouseDown(within(breakdown).getByRole("tab", { name: view }), {
        button: 0,
        ctrlKey: false,
      });
      fireEvent.click(screen.getByRole("button", { name: COPY_STATS_LABEL }));
      const text = mocks.copy.mock.calls[0][0] as string;
      expect(text).toContain("Unallocated usage is included.");
      expect(text).toContain("Exclusive task or session attribution is unavailable.");
      expect(text).toContain("Total tokens: 4,000");
      expect(text).toContain("Cost: $2.00");
      expect(text).not.toContain("Total tokens: 100");
      expect(text).not.toContain("Deleted task");
      expect(text).not.toContain("Unavailable session");
    },
  );
});

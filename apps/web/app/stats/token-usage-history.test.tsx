import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TokenUsageRow } from "@/lib/types/http";
import { TokenUsageBreakdown } from "./token-usage-sections";
import { retainedReport, retainedRow } from "./token-usage.test-helpers";

function breakdown(view: "tasks" | "sessions" = "tasks") {
  const deleted = retainedRow({
    key: "old-incarnation",
    provider: "",
    total_tokens: null,
    amounts_decimal: { total_tokens: "9007199254740993", cost_subcents: "9007199254740993" },
  });
  const archived = retainedRow({
    key: "new-incarnation",
    task_available: true,
    session_available: true,
    task_archived: true,
  });
  return renderBreakdown([deleted, archived], view);
}

function renderBreakdown(rows: TokenUsageRow[], view: "tasks" | "sessions") {
  render(
    <TokenUsageBreakdown
      response={retainedReport({ rows, total_rows: rows.length })}
      view={view}
      onViewChange={vi.fn()}
      provider="all"
      onProviderChange={vi.fn()}
      providers={[]}
      onSort={vi.fn()}
      sortKey="cost_subcents"
      sortDirection="desc"
      offset={0}
      limit={200}
      onPageChange={vi.fn()}
    />,
  );
  return screen.getByTestId("token-usage-table");
}

describe("retained breakdown presentation", () => {
  afterEach(cleanup);
  it("keeps retained incarnations distinct and only links the archived live task", () => {
    const table = breakdown();
    expect(within(table).getAllByTestId("token-usage-row")).toHaveLength(2);
    const deleted = within(table).getByText("Deleted task").closest("tr")!;
    expect(within(deleted).queryByRole("link")).toBeNull();
    expect(within(table).getByRole("link", { name: "task-1" }).getAttribute("href")).toBe(
      "/t/task-1",
    );
    expect(within(table).getByText("Archived")).toBeTruthy();
    expect(within(deleted).getByText("9,007,199,254,740,993", { selector: "td" })).toBeTruthy();
    expect(within(deleted).getByText("$900,719,925,474.0993", { selector: "td" })).toBeTruthy();
  });

  it("labels an unavailable session and exposes independent actor, provider, client and calendar evidence", () => {
    const table = breakdown("sessions");
    const deleted = within(table).getByText("Unavailable session").closest("tr")!;
    expect(within(deleted).queryByRole("link")).toBeNull();
    expect(within(table).getByRole("link", { name: "session-1" }).getAttribute("href")).toBe(
      "/t/task-1?sessionId=session-1",
    );
    const details = within(deleted).getByTestId("token-usage-details");
    for (const text of [
      "Token user attribution",
      "Cost user attribution",
      "Mixed users",
      "Unknown",
      "agent-client-a",
      "exact-model-id",
      "native",
      "collector-a",
      "Asia/Kathmandu",
      "Source calendar. Exact rebucketing is unavailable.",
      "Reported cost",
    ]) {
      expect(within(details).getAllByText(text).length).toBeGreaterThan(0);
    }
    expect(
      within(details).getByText("Effective cost / 1M tokens").nextElementSibling?.textContent,
    ).toBe("Unavailable");
    const phone = screen.getAllByTestId("token-usage-mobile-row")[0];
    expect(within(phone).getByTestId("token-usage-details").textContent).toContain("Mixed users");
    expect(within(phone).getByTestId("token-usage-details").textContent).toContain(
      "9,007,199,254,740,993",
    );
  });
});

describe("unallocated usage presentation", () => {
  afterEach(cleanup);
  it.each(["tasks", "sessions"] as const)(
    "preserves unallocated amounts and distinct %s identities on desktop and phone",
    (view) => {
      const table = renderBreakdown(
        [
          retainedRow({
            key: "shared-work",
            subject_id: "",
            task_id: "",
            session_id: "",
            total_tokens: 2_000,
          }),
          retainedRow({ key: "deleted-work" }),
          retainedRow({
            key: "live-work",
            task_id: "task-live",
            session_id: "session-live",
            task_available: true,
            session_available: true,
          }),
          retainedRow({
            key: "archived-work",
            task_id: "task-archive",
            session_id: "session-archive",
            task_available: true,
            session_available: true,
            task_archived: true,
          }),
        ],
        view,
      );
      expect(within(table).getAllByTestId("token-usage-row")).toHaveLength(4);
      const unallocated = within(table).getByText("Unallocated usage").closest("tr")!;
      const phoneRows = screen.getAllByTestId("token-usage-mobile-row");
      expect(phoneRows).toHaveLength(4);
      for (const surface of [unallocated, phoneRows[0]]) {
        expect(within(surface).queryByRole("link", { hidden: true })).toBeNull();
        expect(within(surface).getAllByText("2,000").length).toBeGreaterThan(0);
        expect(within(surface).getAllByText("$1.2345").length).toBeGreaterThan(0);
        expect(within(surface).getByLabelText("Details for Unallocated usage")).toBeTruthy();
        expect(surface.textContent).not.toContain("Deleted task");
        expect(surface.textContent).not.toContain("Unavailable session");
      }
      const unavailableLabel = view === "tasks" ? "Deleted task" : "Unavailable session";
      const deleted = within(table).getByText(unavailableLabel).closest("tr")!;
      expect(within(deleted).queryByRole("link", { hidden: true })).toBeNull();
      expect(within(phoneRows[1]).getByText(unavailableLabel)).toBeTruthy();
      expect(within(phoneRows[1]).queryByRole("link", { hidden: true })).toBeNull();
      for (const [index, suffix] of [
        [2, "live"],
        [3, "archive"],
      ] as const) {
        const label = `${view === "tasks" ? "task" : "session"}-${suffix}`;
        const destination =
          view === "tasks" ? `/t/task-${suffix}` : `/t/task-${suffix}?sessionId=session-${suffix}`;
        for (const surface of [table, phoneRows[index]]) {
          expect(
            within(surface).getByRole("link", { name: label, hidden: true }).getAttribute("href"),
          ).toBe(destination);
        }
      }
      expect(within(table).getByText("Archived")).toBeTruthy();
      expect(within(phoneRows[3]).getByText("Archived")).toBeTruthy();
    },
  );
});

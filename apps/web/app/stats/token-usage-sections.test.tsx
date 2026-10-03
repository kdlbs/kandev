import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { TokenUsageRow } from "@/lib/types/http";

const tabMocks = vi.hoisted(() => ({
  current: null as null | ((value: string) => void),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@kandev/ui/card", () => ({
  Card: ({ children, ...props }: { children: ReactNode } & Record<string, unknown>) => (
    <section {...props}>{children}</section>
  ),
  CardHeader: ({ children }: { children: ReactNode }) => <header>{children}</header>,
  CardTitle: ({ children }: { children: ReactNode }) => <h2>{children}</h2>,
  CardContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@kandev/ui/tabs", () => {
  return {
    Tabs: ({
      onValueChange,
      children,
    }: {
      onValueChange?: (value: string) => void;
      children: ReactNode;
    }) => {
      tabMocks.current = onValueChange ?? null;
      return <div>{children}</div>;
    },
    TabsList: ({ children }: { children: ReactNode }) => <div>{children}</div>,
    TabsTrigger: ({ value, children }: { value: string; children: ReactNode }) => (
      <button type="button" role="tab" onClick={() => tabMocks.current?.(value)}>
        {children}
      </button>
    ),
  };
});

vi.mock("@kandev/ui/select", () => ({
  Select: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  SelectContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  SelectItem: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  SelectTrigger: ({ children }: { children: ReactNode }) => <button>{children}</button>,
  SelectValue: () => null,
}));

vi.mock("./token-usage-table", () => ({ TokenUsageTables: () => null }));

import { TokenUsageTrend } from "./token-usage-sections";

function usageRow(): TokenUsageRow {
  return {
    key: "usage-1",
    period: "2026-09-13",
    model: "model-a",
    provider: "provider-a",
    task_id: "task-1",
    session_id: "session-1",
    input_tokens: 10,
    output_tokens: 5,
    cache_read_tokens: 0,
    cache_write_tokens: 0,
    reasoning_tokens: 0,
    total_tokens: 15,
    turns: 1,
    cost_subcents: 100,
    currency: "USD",
    cost_basis: "reported",
    cost_coverage: "complete",
    effective_cost_per_million: null,
    source: "plugin:test",
    coverage: "complete",
    estimated: false,
    stale: false,
    undated: false,
  };
}

describe("TokenUsageTrend", () => {
  it("keeps dated points when switching to monthly granularity", () => {
    render(<TokenUsageTrend rows={[usageRow()]} currency="USD" datedCoverage />);

    fireEvent.click(screen.getByRole("tab", { name: "stats:monthly" }));

    expect(screen.getByRole("img", { name: "stats:usageOverTime" })).toBeTruthy();
    expect(screen.queryByText("stats:noTokenUsageInRange")).toBeNull();
  });
});

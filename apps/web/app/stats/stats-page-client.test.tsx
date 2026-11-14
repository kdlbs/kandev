import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  sections: undefined as unknown,
  retrySection: vi.fn(),
  replace: vi.fn(),
  copy: vi.fn(),
}));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ replace: mocks.replace }),
  usePathname: () => "/stats",
  useSearchParams: () => new URLSearchParams(),
}));

vi.mock("@/hooks/use-copy-to-clipboard", () => ({
  useCopyToClipboard: () => ({ copied: false, copy: mocks.copy }),
}));

vi.mock("@/components/page-shell", () => ({
  PageShell: ({ title, subtitle, actions, children }: Record<string, React.ReactNode>) => (
    <section>
      <h1>{title}</h1>
      {subtitle && <p>{subtitle}</p>}
      {actions}
      {children}
    </section>
  ),
}));

vi.mock("@kandev/ui/card", () => ({
  Card: ({ children, ...props }: Record<string, unknown>) => (
    <article {...props}>{children as React.ReactNode}</article>
  ),
  CardHeader: ({ children }: Record<string, React.ReactNode>) => <header>{children}</header>,
  CardTitle: ({ children }: Record<string, React.ReactNode>) => <h2>{children}</h2>,
  CardContent: ({ children }: Record<string, React.ReactNode>) => <div>{children}</div>,
}));

vi.mock("@kandev/ui/button", () => ({
  Button: ({ children, ...props }: Record<string, unknown>) => (
    <button {...props}>{children as React.ReactNode}</button>
  ),
}));

vi.mock("@kandev/ui/toggle-group", () => ({
  ToggleGroup: ({ children }: Record<string, React.ReactNode>) => <div>{children}</div>,
  ToggleGroupItem: ({ children }: Record<string, React.ReactNode>) => <span>{children}</span>,
}));

vi.mock("./stats-sections", () => ({
  OverviewCards: ({ git_stats }: { git_stats?: unknown }) => (
    <div>Overview data {git_stats ? "with git" : "without git"}</div>
  ),
  WorkloadSection: () => <div>Workload data</div>,
  RepositoryStatsGrid: () => <div>Repository data</div>,
  TopRepositories: () => <div>Top repositories data</div>,
  RepoLeaders: () => <div>Repository leaders data</div>,
}));

vi.mock("./stats-charts", () => ({
  ActivityHeatmap: () => <div>Activity data</div>,
  ModelUsageList: () => <div>Model data</div>,
  CompletedTasksChart: () => <div>Completed data</div>,
  MostProductiveSummary: () => <div>Productive data</div>,
}));

vi.mock("./stats-skeletons", () => ({
  ActivitySkeleton: () => <div>Activity loading</div>,
  ChartsSkeleton: () => <div>Charts loading</div>,
  OverviewCardsSkeleton: () => <div>Overview loading</div>,
  RepoLeadersSkeleton: () => <div>Leaders loading</div>,
  RepositoriesSkeleton: () => <div>Repositories loading</div>,
  TopRepositoriesSkeleton: () => <div>Top repositories loading</div>,
  WorkloadSkeleton: () => <div>Workload loading</div>,
}));

vi.mock("@/components/github/pr-stats", () => ({
  PRStatsPanel: () => <div>Pull request data</div>,
}));

vi.mock("./stats-data", async () => {
  const actual = await vi.importActual<typeof import("./stats-data")>("./stats-data");
  return { ...actual, useStatsSections: vi.fn(() => mocks.sections) };
});

import { StatsPageClient } from "./stats-page-client";

const globalData = {
  total_tasks: 1,
  completed_tasks: 1,
  in_progress_tasks: 0,
  total_sessions: 1,
  total_turns: 1,
  total_messages: 1,
  total_user_messages: 1,
  total_tool_calls: 0,
  total_duration_ms: 1000,
  avg_turns_per_task: 1,
  avg_messages_per_task: 1,
  avg_duration_ms_per_task: 1000,
  avg_turn_duration_ms: 1000,
  avg_messages_per_turn: 1,
};

function readySections() {
  return {
    global: { kind: "ready", data: globalData },
    tasks: { kind: "ready", data: { task_stats: [], task_stats_has_more: false } },
    daily: { kind: "ready", data: [] },
    completed: { kind: "ready", data: [] },
    models: { kind: "ready", data: [] },
    repos: { kind: "ready", data: [] },
    git: {
      kind: "ready",
      data: { total_commits: 0, total_files_changed: 0, total_insertions: 0, total_deletions: 0 },
    },
    retrySection: mocks.retrySection,
  };
}

describe("StatsPageClient", () => {
  afterEach(() => cleanup());

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.sections = {
      ...readySections(),
      daily: {
        kind: "error",
        message: "Statistics are temporarily unavailable. Try again.",
        retryable: true,
        data: [],
      },
    };
  });

  it("keeps a retained section visible and exposes an accessible retry", () => {
    render(<StatsPageClient workspaceId="ws-1" activeRange="month" />);

    expect(screen.getByRole("status").textContent).toContain(
      "Statistics are temporarily unavailable",
    );
    expect(screen.getByText("Activity data")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Retry" }).hasAttribute("disabled")).toBe(false);

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(mocks.retrySection).toHaveBeenCalledWith("daily");
    expect(screen.getByRole("button", { name: "Copy Stats" }).hasAttribute("disabled")).toBe(true);
  });

  it("disables the retry while the section request is active", () => {
    mocks.sections = {
      ...(mocks.sections as Record<string, unknown>),
      daily: {
        kind: "error",
        message: "Statistics are temporarily unavailable. Try again.",
        retryable: true,
        retrying: true,
        data: [],
      },
    };

    render(<StatsPageClient workspaceId="ws-1" activeRange="month" />);

    expect(screen.getByRole("button", { name: "Retry" }).hasAttribute("disabled")).toBe(true);
    expect(
      screen.getAllByRole("status").some((status) => status.textContent?.includes("Retrying…")),
    ).toBe(true);
  });

  it("keeps retained Git data visible while the Git refresh fails", () => {
    const sections = readySections() as Record<string, unknown>;
    sections.git = {
      kind: "error",
      message: "Git activity is temporarily unavailable. Try again.",
      retryable: true,
      data: {
        total_commits: 2,
        total_files_changed: 3,
        total_insertions: 4,
        total_deletions: 1,
      },
    };
    mocks.sections = sections;

    render(<StatsPageClient workspaceId="ws-1" activeRange="month" />);

    expect(screen.getByText("Overview data with git")).toBeTruthy();
    expect(screen.getByText("Git activity is temporarily unavailable. Try again.")).toBeTruthy();
  });

  it("enables Copy Stats only after all current sections are ready", () => {
    const { rerender } = render(<StatsPageClient workspaceId="ws-1" activeRange="month" />);
    expect(screen.getByRole("button", { name: "Copy Stats" }).hasAttribute("disabled")).toBe(true);

    mocks.sections = readySections();
    rerender(<StatsPageClient workspaceId="ws-1" activeRange="month" />);
    expect(screen.getByRole("button", { name: "Copy Stats" }).hasAttribute("disabled")).toBe(false);
  });
});

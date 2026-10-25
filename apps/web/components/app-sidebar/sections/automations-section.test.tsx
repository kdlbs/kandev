import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Automation } from "@/lib/types/automation";
import { LIVE_REFRESH_INTERVAL_MS } from "@/components/runs/use-live-refresh";

const WORKSPACE_ID = "workspace-1";
const AUTOMATION_ID = "automation-1";
const AUTOMATIONS_SECTION = "automations";
const OPEN_AUTOMATIONS = "Open automations";
const EMPTY_ROW = "sidebar-automations-empty";
const COLLAPSED_SUMMARY = "sidebar-section-collapsed-summary";

const mocks = vi.hoisted(() => ({
  listAutomations: vi.fn(),
  listAutomationSummaries: vi.fn(),
  activeWorkspaceId: { current: "workspace-1" as string | undefined },
  sectionExpanded: { current: {} as Record<string, boolean> },
  responsive: { isMobile: false },
  toggleSection: vi.fn(),
  setCollapsed: vi.fn(),
}));

type MockState = {
  workspaces: { activeId: string | undefined };
  appSidebar: { sectionExpanded: Record<string, boolean> };
  toggleAppSidebarSection: typeof mocks.toggleSection;
  setAppSidebarCollapsed: typeof mocks.setCollapsed;
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) =>
    selector({
      workspaces: { activeId: mocks.activeWorkspaceId.current },
      appSidebar: { sectionExpanded: mocks.sectionExpanded.current },
      toggleAppSidebarSection: mocks.toggleSection,
      setAppSidebarCollapsed: mocks.setCollapsed,
    }),
}));

vi.mock("@/lib/api/domains/automation-api", () => ({
  listAutomations: mocks.listAutomations,
  listAutomationSummaries: mocks.listAutomationSummaries,
}));

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => `/automations/${AUTOMATION_ID}`,
  useRouter: () => ({ push: vi.fn() }),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => mocks.responsive,
}));

import { AutomationsSection } from "./automations-section";

function mkAutomation(overrides: Partial<Automation> = {}): Automation {
  return {
    id: AUTOMATION_ID,
    workspace_id: WORKSPACE_ID,
    name: "Nightly drift",
    enabled: true,
    max_concurrent_runs: 1,
    last_triggered_at: null,
    created_at: "2026-07-01T00:00:00Z",
    updated_at: "2026-07-01T00:00:00Z",
    triggers: [],
    ...overrides,
  } as unknown as Automation;
}

function renderSection(collapsed = false) {
  return render(
    <TooltipProvider>
      <AutomationsSection collapsed={collapsed} />
    </TooltipProvider>,
  );
}

/** The section is folded until asked for, so any test about the rows opens it. */
function renderOpenSection() {
  mocks.sectionExpanded.current = { [AUTOMATIONS_SECTION]: true };
  return renderSection();
}

beforeEach(() => {
  mocks.activeWorkspaceId.current = WORKSPACE_ID;
  mocks.sectionExpanded.current = {};
  mocks.responsive.isMobile = false;
  mocks.listAutomations.mockReset();
  mocks.listAutomationSummaries.mockReset();
  mocks.listAutomations.mockResolvedValue([mkAutomation()]);
  mocks.listAutomationSummaries.mockResolvedValue([]);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("AutomationsSection", () => {
  it("lists the workspace's automations, each linking to its own history", async () => {
    renderOpenSection();

    const row = await screen.findByTestId(`sidebar-automation-${AUTOMATION_ID}`);
    expect(row.getAttribute("href")).toBe(`/automations/${AUTOMATION_ID}`);
    expect(row.textContent).toContain("Nightly drift");
  });

  it("starts folded — automations run whether or not anyone is watching them", async () => {
    // They should not push the tasks someone came here to work on off the
    // bottom of the rail.
    renderSection();

    await waitFor(() => expect(screen.getByText("Automations")).toBeTruthy());
    expect(screen.queryByTestId(`sidebar-automation-${AUTOMATION_ID}`)).toBeNull();
    expect(screen.queryByRole("link", { name: OPEN_AUTOMATIONS })).toBeNull();
  });

  it("says how many it is hiding while folded, so it does not read as empty", async () => {
    mocks.listAutomations.mockResolvedValue([
      mkAutomation(),
      mkAutomation({ id: "automation-2", name: "Dependency audit" }),
    ]);

    renderSection();

    const summary = await screen.findByTestId(COLLAPSED_SUMMARY);
    expect(summary.textContent).toBe("2");
  });

  it("drops the count once the section is open, where the rows speak for themselves", async () => {
    renderOpenSection();

    await screen.findByTestId(`sidebar-automation-${AUTOMATION_ID}`);
    expect(screen.queryByTestId(COLLAPSED_SUMMARY)).toBeNull();
  });

  it("shows when each automation last ran, in the runs rail's own phrasing", async () => {
    mocks.listAutomationSummaries.mockResolvedValue([
      {
        automation_id: AUTOMATION_ID,
        open_runs: 0,
        last_run: {
          id: "run-1",
          automation_id: AUTOMATION_ID,
          status: "succeeded",
          created_at: new Date(Date.now() - 2 * 60_000).toISOString(),
        },
      },
    ]);

    renderOpenSection();

    const age = await screen.findByTestId(`sidebar-automation-last-run-${AUTOMATION_ID}`);
    expect(age.textContent).toBe("2m ago");
  });

  it("says nothing about a last run for an automation that has never run", async () => {
    renderOpenSection();

    await screen.findByTestId(`sidebar-automation-${AUTOMATION_ID}`);
    expect(screen.queryByTestId(`sidebar-automation-last-run-${AUTOMATION_ID}`)).toBeNull();
  });

  it("marks the automation currently being read", async () => {
    renderOpenSection();

    const row = await screen.findByTestId(`sidebar-automation-${AUTOMATION_ID}`);
    // The active treatment is the shared sidebar accent, not a bespoke style.
    expect(row.className).toContain("before:bg-primary");
  });

  it("shows health from the same derivation the runs list uses", async () => {
    mocks.listAutomationSummaries.mockResolvedValue([
      { automation_id: AUTOMATION_ID, open_runs: 1 },
    ]);

    renderOpenSection();

    // Reaching a screen reader matters here: the dot alone carries the state.
    await waitFor(() => expect(screen.getByText("Running.")).toBeTruthy());
  });

  it("reads a disabled automation as paused, not idle", async () => {
    mocks.listAutomations.mockResolvedValue([mkAutomation({ enabled: false })]);

    renderOpenSection();

    await waitFor(() => expect(screen.getByText("Paused.")).toBeTruthy());
  });

  it("invites setup when the workspace has no automations", async () => {
    mocks.listAutomations.mockResolvedValue([]);

    renderOpenSection();

    const empty = await screen.findByTestId(EMPTY_ROW);
    expect(empty.getAttribute("href")).toBe("/settings/automations");
    expect(screen.getByRole("link", { name: OPEN_AUTOMATIONS }).getAttribute("href")).toBe(
      "/automations",
    );
  });

  it("asks for nothing while collapsed to the rail", async () => {
    // The sidebar is mounted on every page, so an off-screen list must not cost
    // two requests per navigation.
    renderSection(true);

    await waitFor(() => expect(screen.getByLabelText("Automations")).toBeTruthy());
    expect(mocks.listAutomations).not.toHaveBeenCalled();
    expect(mocks.listAutomationSummaries).not.toHaveBeenCalled();
  });

  it("asks only for the names while the section is folded shut", async () => {
    // The count on the header has to come from somewhere, and the names list is
    // the cheap read. Health summaries buy nothing until the rows are on screen.
    mocks.sectionExpanded.current = { [AUTOMATIONS_SECTION]: false };

    renderSection();

    await waitFor(() => expect(mocks.listAutomations).toHaveBeenCalledWith(WORKSPACE_ID));
    expect(mocks.listAutomationSummaries).not.toHaveBeenCalled();
  });

  it("follows the active workspace", async () => {
    mocks.activeWorkspaceId.current = "workspace-other";

    renderSection();

    await waitFor(() => expect(mocks.listAutomations).toHaveBeenCalledWith("workspace-other"));
  });
});

it("places the labelled cross-automation destination after the expanded rows", async () => {
  renderOpenSection();

  const row = await screen.findByTestId(`sidebar-automation-${AUTOMATION_ID}`);
  const shortcut = screen.getByRole("link", { name: OPEN_AUTOMATIONS });
  expect(shortcut.getAttribute("href")).toBe("/automations");
  expect(shortcut.textContent).toBe(OPEN_AUTOMATIONS);
  expect(shortcut.closest("#sidebar-section-automations")).toBe(
    row.closest("#sidebar-section-automations"),
  );
  expect(row.compareDocumentPosition(shortcut) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
});

describe("AutomationsSection live running state", () => {
  it("refreshes a visible row and swaps its indicator as the run state changes", async () => {
    mocks.listAutomationSummaries
      .mockResolvedValueOnce([{ automation_id: AUTOMATION_ID, open_runs: 0 }])
      .mockResolvedValueOnce([{ automation_id: AUTOMATION_ID, open_runs: 1 }])
      .mockResolvedValueOnce([
        {
          automation_id: AUTOMATION_ID,
          open_runs: 0,
          last_run: {
            id: "run-1",
            automation_id: AUTOMATION_ID,
            status: "succeeded",
            created_at: new Date().toISOString(),
          },
        },
      ]);

    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      renderOpenSection();
      await screen.findByTestId(`sidebar-automation-${AUTOMATION_ID}`);
      await waitFor(() => expect(mocks.listAutomationSummaries).toHaveBeenCalledTimes(1));

      await act(async () => {
        await vi.advanceTimersByTimeAsync(LIVE_REFRESH_INTERVAL_MS);
      });
      const runningIndicator = await screen.findByTestId(
        `sidebar-automation-running-${AUTOMATION_ID}`,
      );
      expect(runningIndicator.classList).toContain("animate-spin");
      expect(runningIndicator.getAttribute("aria-hidden")).toBe("true");
      expect(screen.getByText("Running.")).toBeTruthy();

      await act(async () => {
        await vi.advanceTimersByTimeAsync(LIVE_REFRESH_INTERVAL_MS);
      });
      await waitFor(() => expect(mocks.listAutomationSummaries).toHaveBeenCalledTimes(3));
      expect(screen.queryByTestId(`sidebar-automation-running-${AUTOMATION_ID}`)).toBeNull();
      expect(screen.getByText("Idle.")).toBeTruthy();
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("AutomationsSection responsive visibility", () => {
  it("does not fetch or poll health while the desktop rail is hidden on mobile", async () => {
    mocks.responsive.isMobile = true;
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      renderOpenSection();
      await waitFor(() => expect(screen.getByText("Automations")).toBeTruthy());

      await act(async () => {
        await vi.advanceTimersByTimeAsync(LIVE_REFRESH_INTERVAL_MS * 2);
      });

      expect(screen.queryByTestId(`sidebar-automation-${AUTOMATION_ID}`)).toBeNull();
      expect(mocks.listAutomations).not.toHaveBeenCalled();
      expect(mocks.listAutomationSummaries).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("AutomationsSection empty state", () => {
  it("keeps an empty section folded until the user opens it", () => {
    mocks.listAutomations.mockResolvedValue([]);

    renderSection();

    const header = screen.getByRole("button", { name: "Automations" });
    expect(header.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByTestId(EMPTY_ROW)).toBeNull();
  });
});

describe("AutomationsSection last-run age", () => {
  // Nothing pushes an update for this label — the automation did not change,
  // the clock did. An idle sidebar re-renders only when its data does, so
  // without a tick the age freezes at whatever it read when the section opened.
  // Asserting only the first render would pass against exactly that bug.
  it("keeps the age honest as time passes, with nothing pushing an update", async () => {
    mocks.listAutomationSummaries.mockResolvedValue([
      {
        automation_id: AUTOMATION_ID,
        open_runs: 0,
        last_run: {
          id: "run-1",
          automation_id: AUTOMATION_ID,
          status: "succeeded",
          created_at: new Date(Date.now() - 2 * 60_000).toISOString(),
        },
      },
    ]);

    // Fake timers must be installed BEFORE mount: the ticking interval is
    // created during render, and an interval created under real timers is not
    // controlled by a fake clock installed afterwards. `shouldAdvanceTime`
    // keeps the async mount (fetch → state) resolving normally.
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      renderOpenSection();
      const age = await screen.findByTestId(`sidebar-automation-last-run-${AUTOMATION_ID}`);
      expect(age.textContent).toBe("2m ago");

      // Advance the wall clock only. No refetch, no store update, no re-mount.
      // Wrapped in act because the tick sets state from outside React's own
      // event loop; without it the interval fires but the re-render is never
      // flushed, and the assertion reads the pre-tick DOM.
      await act(async () => {
        vi.setSystemTime(new Date(Date.now() + 3 * 60_000));
        await vi.advanceTimersByTimeAsync(30_000);
      });
      expect(screen.getByTestId(`sidebar-automation-last-run-${AUTOMATION_ID}`).textContent).toBe(
        "5m ago",
      );
    } finally {
      vi.useRealTimers();
    }
  });
});

// @covers AC-OFFICE-AUTOMATION-RUNS-001.10
// Real hooks must update the builtin, independently of catalog requests.
describe("external definitions", () => {
  beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it.each(["create", "rename", "delete"])(
    "updates an external %s while mounted",
    async (change) => {
      mocks.listAutomations.mockResolvedValue(change === "create" ? [] : [mkAutomation()]);
      renderOpenSection();
      await act(async () => {});
      const section = document.getElementById("sidebar-section-automations");
      expect(section).not.toBeNull();
      if (change === "create") expect(screen.getByTestId(EMPTY_ROW)).toBeTruthy();
      else
        expect(screen.getByTestId(`sidebar-automation-${AUTOMATION_ID}`).textContent).toContain(
          "Nightly drift",
        );

      mocks.listAutomations.mockResolvedValue(
        change === "delete" ? [] : [mkAutomation({ name: "External change" })],
      );
      await act(async () => {
        await vi.advanceTimersByTimeAsync(LIVE_REFRESH_INTERVAL_MS);
      });

      expect(document.getElementById("sidebar-section-automations")).toBe(section);
      if (change === "delete")
        expect(screen.queryByTestId(`sidebar-automation-${AUTOMATION_ID}`)).toBeNull();
      else
        expect(screen.getByTestId(`sidebar-automation-${AUTOMATION_ID}`).textContent).toContain(
          "External change",
        );
    },
  );

  // @covers AC-OFFICE-AUTOMATION-RUNS-001.3
  it("updates the folded count without starting health reads or opening the section", async () => {
    renderSection();
    await act(async () => {});
    expect(screen.getByTestId(COLLAPSED_SUMMARY).textContent).toBe("1");
    mocks.listAutomations.mockResolvedValue([mkAutomation(), mkAutomation({ id: "second" })]);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIVE_REFRESH_INTERVAL_MS);
    });
    expect(screen.getByTestId(COLLAPSED_SUMMARY).textContent).toBe("2");
    expect(screen.getByRole("button", { name: /Automations/ }).getAttribute("aria-expanded")).toBe(
      "false",
    );
    expect(mocks.listAutomationSummaries).not.toHaveBeenCalled();
    expect(mocks.toggleSection).not.toHaveBeenCalled();
  });
});

describe("external definitions lifecycle", () => {
  beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });
  // @covers AC-OFFICE-AUTOMATION-RUNS-001.12
  it.each(["focus", "online", "visibilitychange"])(
    "recovers definitions on %s before the next interval",
    async (event) => {
      renderOpenSection();
      await act(async () => {});
      mocks.listAutomations.mockResolvedValue([mkAutomation({ name: "Foreground name" })]);
      await act(async () => {
        if (event === "visibilitychange") document.dispatchEvent(new Event(event));
        else window.dispatchEvent(new Event(event));
      });
      expect(screen.getByTestId(`sidebar-automation-${AUTOMATION_ID}`).textContent).toContain(
        "Foreground name",
      );
    },
  );

  it("retries a failed read on the next interval", async () => {
    mocks.listAutomations.mockRejectedValueOnce(new Error("offline"));
    renderOpenSection();
    await act(async () => {});
    expect(screen.getByTestId(EMPTY_ROW)).toBeTruthy();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIVE_REFRESH_INTERVAL_MS);
    });
    expect(screen.getByTestId(`sidebar-automation-${AUTOMATION_ID}`)).toBeTruthy();
  });

  it("stops definition reads in the rail and after unmount, then reloads on reopen", async () => {
    const view = renderOpenSection();
    await act(async () => {});
    view.rerender(
      <TooltipProvider>
        <AutomationsSection collapsed />
      </TooltipProvider>,
    );
    const calls = mocks.listAutomations.mock.calls.length;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(LIVE_REFRESH_INTERVAL_MS);
    });
    expect(mocks.listAutomations).toHaveBeenCalledTimes(calls);
    mocks.listAutomations.mockResolvedValue([mkAutomation({ name: "Reopened" })]);
    view.rerender(
      <TooltipProvider>
        <AutomationsSection collapsed={false} />
      </TooltipProvider>,
    );
    await act(async () => {});
    expect(screen.getByTestId(`sidebar-automation-${AUTOMATION_ID}`).textContent).toContain(
      "Reopened",
    );
    view.unmount();
    const finalCalls = mocks.listAutomations.mock.calls.length;
    await act(async () => {
      window.dispatchEvent(new Event("focus"));
      await vi.advanceTimersByTimeAsync(LIVE_REFRESH_INTERVAL_MS);
    });
    expect(mocks.listAutomations).toHaveBeenCalledTimes(finalCalls);
  });
});

describe("slow external definitions", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("lets slow reads publish before resuming automatic refresh", async () => {
    let release!: (rows: Automation[]) => void;
    const releases: Array<(rows: Automation[]) => void> = [];
    mocks.listAutomations.mockImplementation(
      () =>
        new Promise<Automation[]>((resolve) => {
          release = resolve;
          releases.push(resolve);
        }),
    );
    renderOpenSection();
    try {
      for (const name of ["Slow initial read", "Slow refreshed name"]) {
        const calls = mocks.listAutomations.mock.calls.length;
        await act(async () => {
          window.dispatchEvent(new Event("focus"));
          await vi.advanceTimersByTimeAsync(LIVE_REFRESH_INTERVAL_MS * 2);
        });
        expect(mocks.listAutomations).toHaveBeenCalledTimes(calls);
        await act(async () => release([mkAutomation({ name })]));
        expect(screen.getByTestId(`sidebar-automation-${AUTOMATION_ID}`).textContent).toContain(
          name,
        );
        await act(async () => vi.advanceTimersByTimeAsync(LIVE_REFRESH_INTERVAL_MS));
        expect(mocks.listAutomations).toHaveBeenCalledTimes(calls + 1);
      }
    } finally {
      await act(async () => releases.forEach((resolve) => resolve([])));
    }
  });
});

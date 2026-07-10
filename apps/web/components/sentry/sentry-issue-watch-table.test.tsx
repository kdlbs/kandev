import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { SentryIssueWatch } from "@/lib/types/sentry";
import { SentryIssueWatchTable } from "./sentry-issue-watch-table";

function watch(over: Partial<SentryIssueWatch>): SentryIssueWatch {
  return {
    id: "w1",
    workspaceId: "ws-1",
    sentryInstanceId: "inst-1",
    workflowId: "wf",
    workflowStepId: "step",
    repositoryId: "",
    baseBranch: "",
    filter: { orgSlug: "acme", projectSlug: "web" },
    agentProfileId: "ap",
    executorProfileId: "",
    prompt: "",
    enabled: true,
    pollIntervalSeconds: 300,
    lastPolledAt: null,
    lastError: "",
    lastErrorAt: null,
    createdAt: "",
    updatedAt: "",
    ...over,
  } as unknown as SentryIssueWatch;
}

const noop = vi.fn();

function resolveName(id: string): string {
  if (id === "inst-1") return "Production";
  if (id === "") return "—";
  return "(unavailable)";
}

function renderTable(watches: SentryIssueWatch[]) {
  return render(
    <TooltipProvider>
      <SentryIssueWatchTable
        watches={watches}
        instanceName={resolveName}
        onEdit={noop}
        onDelete={noop}
        onTrigger={noop}
        onReset={noop}
        onToggleEnabled={noop}
      />
    </TooltipProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("SentryIssueWatchTable", () => {
  it("shows an Instance column, not a Workspace column (integration is workspace-scoped)", () => {
    renderTable([watch({ id: "w1" })]);
    expect(screen.getByText("Instance")).toBeTruthy();
    expect(screen.queryByText("Workspace")).toBeNull();
  });

  it("renders each watch's bound instance name via the resolver", () => {
    renderTable([
      watch({ id: "w1", sentryInstanceId: "inst-1" }),
      watch({ id: "w2", sentryInstanceId: "inst-gone" }),
      watch({ id: "w3", sentryInstanceId: "" }),
    ]);
    const cells = screen.getAllByTestId("watch-instance").map((c) => c.textContent);
    expect(cells).toEqual(["Production", "(unavailable)", "—"]);
  });
});

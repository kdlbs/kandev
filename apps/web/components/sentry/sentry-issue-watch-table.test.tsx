import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { SentryIssueWatch } from "@/lib/types/sentry";
import { SentryIssueWatchTable } from "./sentry-issue-watch-table";

const responsive = vi.hoisted(() => ({ isFinePointer: true }));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsive,
}));

function watch(over: Partial<SentryIssueWatch>): SentryIssueWatch {
  return {
    id: "w1",
    workspaceId: "ws-1",
    sentryInstanceId: "inst-1",
    workflowId: "wf",
    workflowStepId: "step",
    repositoryId: "",
    baseBranch: "",
    filter: { orgSlug: "acme", projectSlugs: ["web"] },
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

function renderTable(watches: SentryIssueWatch[], onDelete = noop) {
  return render(
    <TooltipProvider>
      <SentryIssueWatchTable
        watches={watches}
        dirtyIds={new Set(watches.slice(0, 1).map(({ id }) => id))}
        instanceName={resolveName}
        onEdit={noop}
        onDelete={onDelete}
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
  Object.defineProperty(window, "confirm", { configurable: true, value: undefined });
});

function installNativeConfirm() {
  const nativeConfirm = vi.fn();
  Object.defineProperty(window, "confirm", {
    configurable: true,
    value: nativeConfirm,
  });
  return nativeConfirm;
}

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

  it("marks the changed watcher row and enable control dirty", () => {
    renderTable([watch({ id: "w1" }), watch({ id: "w2" })]);

    expect(screen.getByTestId("sentry-watch-row-w1").getAttribute("data-settings-dirty")).toBe(
      "true",
    );
    expect(screen.getByTestId("sentry-watch-enabled-w1").getAttribute("data-settings-dirty")).toBe(
      "true",
    );
    expect(screen.getByTestId("sentry-watch-row-w2").getAttribute("data-settings-dirty")).toBe(
      "false",
    );
  });

  it("renders a comma-joined project list in the filter summary for a multi-project watch", () => {
    renderTable([
      watch({ id: "w1", filter: { orgSlug: "acme", projectSlugs: ["frontend", "backend"] } }),
    ]);
    expect(screen.getByText("org:acme · project:frontend,backend")).toBeTruthy();
  });

  it("anchors delete confirmation to the row action without using native confirm", async () => {
    const onDelete = vi.fn();
    const nativeConfirm = installNativeConfirm();
    renderTable([watch({ id: "w1" })], onDelete);

    fireEvent.click(screen.getByTestId("sentry-watch-delete-w1"));

    const confirmation = await screen.findByRole("dialog", {
      name: "Delete this Sentry watcher?",
    });
    expect(nativeConfirm).not.toHaveBeenCalled();
    fireEvent.click(within(confirmation).getByRole("button", { name: "Delete" }));

    await waitFor(() => expect(onDelete).toHaveBeenCalledWith("w1", "ws-1"));
  });

  it("morphs coarse-pointer delete into one 44px inline confirmation row", async () => {
    responsive.isFinePointer = false;
    const onDelete = vi.fn();
    render(
      <TooltipProvider>
        <SentryIssueWatchTable
          watches={[watch({ id: "w1" })]}
          dirtyIds={new Set()}
          instanceName={resolveName}
          onEdit={noop}
          onDelete={onDelete}
          onTrigger={noop}
          onReset={noop}
          onToggleEnabled={noop}
        />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByTestId("sentry-watch-delete-w1"));

    const confirmation = await screen.findByRole("group", {
      name: "Delete this Sentry watcher?",
    });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(within(confirmation).getByRole("button", { name: "Cancel" }).className).toContain(
      "h-11",
    );
    expect(within(confirmation).getByRole("button", { name: "Delete" }).className).toContain(
      "min-w-11",
    );

    fireEvent.click(within(confirmation).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(onDelete).toHaveBeenCalledWith("w1", "ws-1"));
  });
});

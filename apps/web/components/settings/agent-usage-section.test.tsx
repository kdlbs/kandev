/* eslint-disable sonarjs/no-duplicate-string */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { QueryClientProvider } from "@tanstack/react-query";
import { makeQueryClient } from "@/lib/query/client";
import { AgentUsageSection } from "@/components/settings/agent-usage-section";

const listAgentSubscriptionUsage = vi.hoisted(() =>
  vi.fn<typeof import("@/lib/api/domains/settings-api").listAgentSubscriptionUsage>(),
);

vi.mock("@/lib/api/domains/settings-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/settings-api")>(
    "@/lib/api/domains/settings-api",
  );
  return { ...actual, listAgentSubscriptionUsage };
});

vi.mock("@/components/agent-logo", () => ({
  AgentLogo: ({ agentName }: { agentName: string }) => <span data-testid={`logo-${agentName}`} />,
}));

afterEach(() => {
  cleanup();
  listAgentSubscriptionUsage.mockReset();
});

describe("AgentUsageSection", () => {
  function renderSection() {
    return render(
      <QueryClientProvider client={makeQueryClient()}>
        <TooltipProvider>
          <AgentUsageSection />
        </TooltipProvider>
      </QueryClientProvider>,
    );
  }

  it("renders nothing when no subscription agents are present", async () => {
    listAgentSubscriptionUsage.mockResolvedValue({ agents: [] });

    const { container } = renderSection();

    await waitFor(() => expect(listAgentSubscriptionUsage).toHaveBeenCalled());
    expect(container.querySelector('[data-testid="agent-usage-section"]')).toBeNull();
  });

  it("renders utilization windows, plan badge, and per-agent errors", async () => {
    listAgentSubscriptionUsage.mockResolvedValue({
      agents: [
        {
          agent_id: "claude-acp",
          display_name: "Claude Code",
          usage: {
            provider: "anthropic",
            plan: "max",
            windows: [
              {
                label: "5-hour",
                utilization_pct: 62,
                reset_at: new Date(Date.now() + 3600_000).toISOString(),
              },
              {
                label: "7-day",
                utilization_pct: 15,
                reset_at: new Date(Date.now() + 86400_000).toISOString(),
              },
            ],
            fetched_at: new Date().toISOString(),
          },
        },
        {
          agent_id: "codex-acp",
          display_name: "Codex",
          error: "codex usage: unexpected status 500",
        },
      ],
    });

    renderSection();

    await screen.findByTestId("agent-usage-section");
    expect(screen.getByText("Claude Code")).toBeDefined();
    expect(screen.getByText("max")).toBeDefined();
    expect(screen.getByText("Good")).toBeDefined(); // worst window 62% → Good
    expect(screen.getByText("5h")).toBeDefined();
    expect(screen.getByText("62%")).toBeDefined();
    expect(screen.getByText("7d")).toBeDefined();
    expect(screen.getByText("Codex")).toBeDefined();
    expect(screen.getByText("Could not fetch usage data.")).toBeDefined();
  });

  it("keeps cached usage visible when a manual refresh fails", async () => {
    listAgentSubscriptionUsage
      .mockResolvedValueOnce({
        agents: [{ agent_id: "claude-acp", display_name: "Claude Code", error: "unavailable" }],
      })
      .mockRejectedValueOnce(new Error("refresh failed"));

    renderSection();
    await screen.findByText("Claude Code");

    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));

    await waitFor(() => expect(listAgentSubscriptionUsage).toHaveBeenCalledTimes(2));
    expect(screen.getByText("Claude Code")).toBeDefined();
  });
});

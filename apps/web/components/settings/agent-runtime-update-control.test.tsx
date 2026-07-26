import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ComponentType } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { InstalledAgentCard } from "./installed-agent-card";

const ManagedCard = InstalledAgentCard as ComponentType<Record<string, unknown>>;
const AGENT_NAME = "claude-acp";

const discoveryAgent = {
  name: AGENT_NAME,
  supports_mcp: true,
  installation_paths: [],
  available: true,
  matched_path: "/usr/bin/npx",
};

const runtimeUpdate = {
  supported: true,
  package: "@agentclientprotocol/claude-agent-acp",
  current_version: "1.0.0",
};

function renderCard(overrides: Record<string, unknown> = {}) {
  const onUpdate = vi.fn();
  render(
    <ManagedCard
      agent={discoveryAgent}
      savedAgent={undefined}
      displayName="Claude"
      runtimeUpdate={runtimeUpdate}
      onUpdate={onUpdate}
      {...overrides}
    />,
  );
  return onUpdate;
}

afterEach(cleanup);

describe("AgentRuntimeUpdateControl", () => {
  it("shows a labeled touch action only for managed installed agents", () => {
    const onUpdate = renderCard();

    const button = screen.getByRole("button", { name: "Update agent" });
    expect(button.className).toContain("min-h-11");
    fireEvent.click(button);
    expect(onUpdate).toHaveBeenCalledWith(AGENT_NAME);

    cleanup();
    renderCard({ runtimeUpdate: undefined });
    expect(screen.queryByRole("button", { name: "Update agent" })).toBeNull();
  });

  it("shows current to target progress and a bounded wrapping log", () => {
    renderCard({
      updateJob: {
        job_id: "update-1",
        agent_name: AGENT_NAME,
        status: "updating",
        current_version: "1.0.0",
        target_version: "1.1.0",
        output: "fetching package\n",
        started_at: "2026-07-26T10:00:00Z",
      },
    });

    expect(screen.getByText("1.0.0 → 1.1.0")).not.toBeNull();
    expect(screen.getByText("Updating runtime…")).not.toBeNull();
    const log = screen.getByTestId("agent-update-log-claude-acp");
    expect(log.className).toContain("max-h-40");
    expect(log.className).toContain("overflow-y-auto");
    expect(log.className).toContain("break-words");
    expect(screen.getByRole("button", { name: "Updating agent" }).hasAttribute("disabled")).toBe(
      true,
    );
  });

  it("distinguishes refresh warnings from retryable update failures", () => {
    const { rerender } = render(
      <ManagedCard
        agent={discoveryAgent}
        savedAgent={undefined}
        displayName="Claude"
        runtimeUpdate={runtimeUpdate}
        onUpdate={vi.fn()}
        updateJob={{
          job_id: "update-1",
          agent_name: AGENT_NAME,
          status: "succeeded",
          current_version: "1.0.0",
          target_version: "1.1.0",
          refresh_error: "Authentication required",
          started_at: "2026-07-26T10:00:00Z",
        }}
      />,
    );

    expect(
      screen.getByText(/runtime updated, but capabilities could not be refreshed/i),
    ).not.toBeNull();
    expect(screen.getByText(/authentication required/i)).not.toBeNull();

    rerender(
      <ManagedCard
        agent={discoveryAgent}
        savedAgent={undefined}
        displayName="Claude"
        runtimeUpdate={runtimeUpdate}
        onUpdate={vi.fn()}
        updateJob={{
          job_id: "update-2",
          agent_name: AGENT_NAME,
          status: "failed",
          current_version: "1.0.0",
          error: "Registry unavailable",
          started_at: "2026-07-26T10:01:00Z",
        }}
      />,
    );

    expect(screen.getByText("Registry unavailable")).not.toBeNull();
    expect(screen.getByRole("button", { name: "Retry update" }).hasAttribute("disabled")).toBe(
      false,
    );
  });

  it("disables updates while the same agent is installing", () => {
    renderCard({
      installJob: {
        job_id: "install-1",
        agent_name: AGENT_NAME,
        status: "running",
        started_at: "2026-07-26T10:00:00Z",
      },
    });

    expect(screen.getByRole("button", { name: "Update agent" }).hasAttribute("disabled")).toBe(
      true,
    );
    expect(screen.getByText("Agent installation is in progress.")).not.toBeNull();
  });
});

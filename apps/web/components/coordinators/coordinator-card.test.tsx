import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import { CoordinatorCard } from "./coordinator-card";

function mkCoordinator(overrides: Partial<Coordinator> = {}): Coordinator {
  return {
    id: "c-1",
    workspace_id: "ws-1",
    name: "Planner",
    agent_profile_id: "agent-1",
    executor_profile_id: "profile-1",
    context: "Relay ships consent features; prefer small cards.",
    conversation_task_id: null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("CoordinatorCard", () => {
  afterEach(cleanup);

  it("shows the coordinator's name, agent profile, executor and context (AC-004.2)", () => {
    render(
      <CoordinatorCard
        coordinator={mkCoordinator()}
        agentProfileLabel="Claude, Sonnet"
        executorProfileLabel="worktree"
        openHref="/workspaces/ws-1/coordinator/c-1"
        configureHref="/settings/workspaces/ws-1/coordinators/c-1"
      />,
    );

    const card = screen.getByTestId("coordinator-card-c-1");
    expect(card.textContent).toContain("Planner");
    expect(card.textContent).toContain("Claude, Sonnet");
    expect(card.textContent).toContain("worktree");
    expect(card.textContent).toContain("Relay ships consent features");
  });

  it("points Open at the coordinator's Needs you href, unconditionally (D7)", () => {
    render(
      <CoordinatorCard
        coordinator={mkCoordinator()}
        agentProfileLabel="Claude, Sonnet"
        executorProfileLabel="worktree"
        openHref="/workspaces/ws-1/coordinator/c-1"
        configureHref="/settings/workspaces/ws-1/coordinators/c-1"
      />,
    );

    expect(screen.getByTestId("coordinator-open-c-1").getAttribute("href")).toBe(
      "/workspaces/ws-1/coordinator/c-1",
    );
  });

  it("points Configure and the name link at the coordinator's settings page", () => {
    render(
      <CoordinatorCard
        coordinator={mkCoordinator()}
        agentProfileLabel="Claude, Sonnet"
        executorProfileLabel="worktree"
        openHref="/workspaces/ws-1/coordinator/c-1"
        configureHref="/settings/workspaces/ws-1/coordinators/c-1"
      />,
    );

    expect(screen.getByTestId("coordinator-configure-c-1").getAttribute("href")).toBe(
      "/settings/workspaces/ws-1/coordinators/c-1",
    );
    expect(screen.getByTestId("coordinator-name-link-c-1").getAttribute("href")).toBe(
      "/settings/workspaces/ws-1/coordinators/c-1",
    );
  });

  it("gives Open and Configure a 44px touch target on phones and coarse pointers (mobile parity regression)", () => {
    render(
      <CoordinatorCard
        coordinator={mkCoordinator()}
        agentProfileLabel="Claude, Sonnet"
        executorProfileLabel="worktree"
        openHref="/workspaces/ws-1/coordinator/c-1"
        configureHref="/settings/workspaces/ws-1/coordinators/c-1"
      />,
    );

    const openLink = screen.getByTestId("coordinator-open-c-1");
    expect(openLink.className).toContain("max-md:h-11");
    expect(openLink.className).toContain("[@media(pointer:coarse)]:h-11");
    expect(openLink.className).not.toContain("h-6");

    const configureLink = screen.getByTestId("coordinator-configure-c-1");
    expect(configureLink.className).toContain("max-md:h-11");
    expect(configureLink.className).toContain("[@media(pointer:coarse)]:h-11");
  });
});

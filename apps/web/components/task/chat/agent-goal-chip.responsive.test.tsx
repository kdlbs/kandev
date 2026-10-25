import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentGoal } from "@/lib/agent-goal";
import { AgentGoalChip } from "./agent-goal-chip";

type Listener = () => void;

const listeners = new Set<Listener>();
let pointer: "fine" | "coarse" = "fine";

function installViewport(): void {
  Object.defineProperty(window, "innerWidth", {
    configurable: true,
    writable: true,
    value: 1024,
  });
  window.matchMedia = vi.fn((query: string) => {
    let matches = false;
    if (query.includes("pointer: fine")) matches = pointer === "fine";
    else if (query.includes("min-width")) matches = true;
    return {
      media: query,
      matches,
      onchange: null,
      addEventListener: vi.fn((_event: string, listener: Listener) => listeners.add(listener)),
      removeEventListener: vi.fn((_event: string, listener: Listener) =>
        listeners.delete(listener),
      ),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    } as unknown as MediaQueryList;
  });
}

function setPointer(next: "fine" | "coarse"): void {
  pointer = next;
  installViewport();
  act(() => {
    for (const listener of listeners) listener();
  });
}

function activeGoal(): AgentGoal {
  return {
    objective: "Coordinate contributor PR reviews",
    status: "active",
    createdAt: 10,
    updatedAt: 20,
  };
}

beforeEach(() => {
  listeners.clear();
  pointer = "fine";
  installViewport();
});

afterEach(() => {
  cleanup();
  listeners.clear();
  vi.restoreAllMocks();
});

describe("AgentGoalChip responsive subscription", () => {
  it("changes disclosure surface when pointer mode changes while mounted", () => {
    render(<AgentGoalChip goal={activeGoal()} />);

    expect(screen.getByTestId("agent-goal-chip").className).toContain("h-6");

    setPointer("coarse");
    expect(screen.getByTestId("agent-goal-chip").className).toContain("min-h-11");

    setPointer("fine");
    expect(screen.getByTestId("agent-goal-chip").className).toContain("h-6");
  });
});

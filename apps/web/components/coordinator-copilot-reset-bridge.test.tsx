import { cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const path = vi.hoisted(() => ({ current: "/workspaces/ws-1/coordinator/co-1" }));
vi.mock("@/lib/routing/client-router", () => ({ usePathname: () => path.current }));

import { useCopilotStore } from "@/hooks/domains/coordinator/copilot-store";
import { CoordinatorCopilotResetBridge } from "./coordinator-copilot-reset-bridge";

const EMPTY = { open: false, chip: null, draft: "" };

beforeEach(() => {
  useCopilotStore.getState().removeEntry("co-1");
  useCopilotStore.getState().askAboutThis("co-1", "KAN-1", "Why?");
});

afterEach(() => {
  cleanup();
  path.current = "/workspaces/ws-1/coordinator/co-1";
});

describe("CoordinatorCopilotResetBridge", () => {
  it("keeps the slot across Needs you and Queue of the same coordinator", () => {
    const { rerender } = render(<CoordinatorCopilotResetBridge />);
    path.current = "/workspaces/ws-1/coordinator/co-1/queue";
    rerender(<CoordinatorCopilotResetBridge />);
    expect(useCopilotStore.getState().getEntry("co-1").chip?.id).toBe("KAN-1");
    expect(useCopilotStore.getState().getEntry("co-1").open).toBe(true);
  });

  it("resets the slot when the path moves to another coordinator", () => {
    const { rerender } = render(<CoordinatorCopilotResetBridge />);
    path.current = "/workspaces/ws-1/coordinator/co-2";
    rerender(<CoordinatorCopilotResetBridge />);
    expect(useCopilotStore.getState().getEntry("co-1")).toEqual(EMPTY);
  });

  it("resets the slot on any non-coordinator path", () => {
    const { rerender } = render(<CoordinatorCopilotResetBridge />);
    path.current = "/workspaces/ws-1/tasks";
    rerender(<CoordinatorCopilotResetBridge />);
    expect(useCopilotStore.getState().getEntry("co-1")).toEqual(EMPTY);
  });
});

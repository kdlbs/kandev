import { vi } from "vitest";
import type { EnvSwitchParams } from "./dockview-env-switch";
import type { getEnvLayout } from "@/lib/local-storage";

export function makeMockApi() {
  return {
    panels: [],
    groups: [],
    layout: vi.fn(),
    fromJSON: vi.fn(),
    getPanel: vi.fn(() => null),
    addPanel: vi.fn(),
  } as unknown as EnvSwitchParams["api"];
}

export function makeHealthyLayoutWith(extraPanels: Record<string, { contentComponent: string }>) {
  return {
    grid: {
      root: {
        type: "leaf" as const,
        size: 800,
        data: { id: "g1", views: ["chat"], activeView: "chat" },
      },
      height: 600,
      width: 800,
      orientation: "HORIZONTAL" as const,
    },
    panels: {
      chat: { contentComponent: "chat" },
      ...extraPanels,
    },
    activeGroup: "g1",
  } as unknown as ReturnType<typeof getEnvLayout>;
}

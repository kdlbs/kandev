import { cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  routerPush: vi.fn(),
  onViewModeChange: vi.fn(),
  commands: [] as Array<{
    id: string;
    label?: string;
    group?: string;
    keywords?: string[];
    action?: () => void;
  }>,
  activeWorkspaceId: "workspace-1" as string | null,
  activeWorkflowId: "workflow-1" as string | null,
}));

// `t` echoes the key it is given, so copy that never reached the catalog comes
// back as plain English and the copy test below names it. Asserting the English
// itself cannot distinguish a catalog hit from an inlined literal.
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => `«${key}»` }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      userSettings: { keyboardShortcuts: {} },
      workspaces: { activeId: mocks.activeWorkspaceId },
      workflows: { activeId: mocks.activeWorkflowId },
    }),
}));
vi.mock("@/hooks/use-kanban-display-settings", () => ({
  useKanbanDisplaySettings: () => ({ onViewModeChange: mocks.onViewModeChange }),
}));
vi.mock("@/hooks/use-register-commands", () => ({
  useRegisterCommands: (commands: Array<{ id: string; action?: () => void }>) => {
    mocks.commands = commands;
  },
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));
vi.mock("@/lib/keyboard/shortcut-overrides", () => ({
  getShortcut: () => undefined,
}));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mocks.routerPush }),
}));

import { HomepageCommands } from "./homepage-commands";

function commandAction(id: string): () => void {
  const action = mocks.commands.find((command) => command.id === id)?.action;
  if (!action) throw new Error(`Missing ${id} command action`);
  return action;
}

beforeEach(() => {
  mocks.routerPush.mockReset();
  mocks.onViewModeChange.mockReset();
  mocks.commands = [];
  mocks.activeWorkspaceId = "workspace-1";
  mocks.activeWorkflowId = "workflow-1";
});

afterEach(() => cleanup());

describe("HomepageCommands", () => {
  it("preserves the active scope when changing views", () => {
    render(<HomepageCommands onCreateTask={vi.fn()} />);

    commandAction("view-kanban")();
    expect(mocks.routerPush).toHaveBeenCalledWith(
      "/?home=overview&workspaceId=workspace-1&workflowId=workflow-1",
    );

    mocks.routerPush.mockClear();
    commandAction("view-pipeline")();
    expect(mocks.routerPush).toHaveBeenCalledWith(
      "/?home=overview&workspaceId=workspace-1&workflowId=workflow-1",
    );

    mocks.routerPush.mockClear();
    commandAction("view-list")();
    expect(mocks.routerPush).toHaveBeenCalledWith("/tasks?workspace=workspace-1");
  });

  it("resolves every registered label and group through the catalog", () => {
    render(<HomepageCommands onCreateTask={vi.fn()} />);

    const inlined = mocks.commands.flatMap((command) =>
      [
        ["label", command.label],
        ["group", command.group],
        // Search keywords are matched, never displayed, but a non-English user
        // types in their own language — leaving them English makes the command
        // unfindable. They arrive as one comma-separated catalog value.
        ["keywords", command.keywords?.join(",")],
      ]
        .filter(([, value]) => typeof value === "string" && !value.startsWith("«"))
        .map(([field, value]) => `${command.id}.${field} = ${value}`),
    );

    expect(inlined).toEqual([]);
    expect(mocks.commands.length).toBeGreaterThan(0);
    expect(mocks.commands.every((command) => (command.keywords?.length ?? 0) > 0)).toBe(true);
  });
});

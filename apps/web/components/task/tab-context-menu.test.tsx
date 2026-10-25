import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";

vi.mock("dockview-react", () => ({
  DockviewDefaultTab: () => null,
}));

vi.mock("./use-tab-maximize", () => ({
  useTabMaximizeOnDoubleClick: () => () => {},
}));

// Render the menu inline so its items are clickable without driving Radix's
// pointer choreography.
vi.mock("@kandev/ui/context-menu", () => ({
  ContextMenu: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  ContextMenuTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  ContextMenuContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  ContextMenuItem: ({
    children,
    onSelect,
    disabled,
  }: {
    children: React.ReactNode;
    onSelect: () => void;
    disabled?: boolean;
  }) => (
    <button type="button" disabled={disabled} onClick={onSelect}>
      {children}
    </button>
  ),
}));

import { ContextMenuTab } from "./tab-context-menu";

type Panel = { id: string };

function makeProps(panelId: string, groupPanels: string[], params?: Record<string, unknown>) {
  const removePanel = vi.fn();
  const panels: Panel[] = groupPanels.map((id) => ({ id }));
  const api = { id: panelId, group: { panels } };
  const containerApi = {
    getPanel: (id: string) => panels.find((panel) => panel.id === id),
    removePanel,
  };
  return {
    removePanel,
    props: { api, containerApi, params } as unknown as React.ComponentProps<typeof ContextMenuTab>,
  };
}

describe("ContextMenuTab", () => {
  afterEach(() => cleanup());

  it("closes its own panel from the menu", () => {
    const { props, removePanel } = makeProps("browser", ["chat", "browser", "terminal"]);
    render(<ContextMenuTab {...props} />);

    fireEvent.click(screen.getByRole("button", { name: "Close" }));

    expect(removePanel).toHaveBeenCalledTimes(1);
    expect(removePanel).toHaveBeenCalledWith({ id: "browser" });
  });

  it("closes siblings but spares the chat and session panels", () => {
    const { props, removePanel } = makeProps("browser", [
      "chat",
      "session:abc",
      "browser",
      "terminal",
      "files",
    ]);
    render(<ContextMenuTab {...props} />);

    fireEvent.click(screen.getByRole("button", { name: "Close Others" }));

    expect(removePanel.mock.calls.map(([panel]) => (panel as Panel).id)).toEqual([
      "terminal",
      "files",
    ]);
  });

  it("renders panel-injected items alongside the close actions", () => {
    const onSelect = vi.fn();
    const { props } = makeProps("browser", ["browser"], {
      contextMenuItems: [{ label: "Reload", onSelect }],
    });
    render(<ContextMenuTab {...props} />);

    fireEvent.click(screen.getByRole("button", { name: "Reload" }));

    expect(onSelect).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: "Close" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Close Others" })).toBeTruthy();
  });
});

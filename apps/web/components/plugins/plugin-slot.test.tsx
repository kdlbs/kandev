import { act, cleanup, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import { PluginSlot } from "./plugin-slot";

const SLOT = "task-sidebar";
const OWNER_SLOT = "plugin-settings";

function cleanupPlugins(...pluginIds: string[]) {
  pluginIds.forEach((id) => pluginRegistry.unregisterPlugin(id));
}

afterEach(() => {
  cleanup();
  cleanupPlugins("plugin-a", "plugin-b", "plugin-c");
});

describe("PluginSlot", () => {
  it("renders nothing when no plugin has registered a component for the slot", () => {
    const { container } = render(<PluginSlot name={SLOT} />);
    expect(container.innerHTML).toBe("");
  });

  it("renders every component registered for the named slot", () => {
    pluginRegistry
      .forPlugin("plugin-a")
      .registerComponent(SLOT, () => <div data-testid="slot-a">A</div>);
    pluginRegistry
      .forPlugin("plugin-b")
      .registerComponent(SLOT, () => <div data-testid="slot-b">B</div>);

    render(<PluginSlot name={SLOT} />);

    expect(screen.getByTestId("slot-a")).not.toBeNull();
    expect(screen.getByTestId("slot-b")).not.toBeNull();
  });

  it("does not render a component registered for a different slot", () => {
    pluginRegistry
      .forPlugin("plugin-a")
      .registerComponent("settings-nav", () => <div data-testid="slot-a">A</div>);

    render(<PluginSlot name={SLOT} />);

    expect(screen.queryByTestId("slot-a")).toBeNull();
  });

  it("passes slotProps through to each registered component", () => {
    pluginRegistry
      .forPlugin("plugin-a")
      .registerComponent(SLOT, ({ slotProps }) => (
        <div data-testid="slot-a">{String((slotProps as { label: string })?.label)}</div>
      ));

    render(<PluginSlot name={SLOT} slotProps={{ label: "hello" }} />);

    expect(screen.getByTestId("slot-a").textContent).toBe("hello");
  });

  it("renders only the owning plugin's component when ownerPluginId is set", () => {
    // "plugin-settings" is owner-scoped: the host isolates by owner, so a
    // registered component never has to gate on the current plugin id itself.
    const Card = ({ slotProps }: { slotProps?: unknown }) => (
      <div data-testid="settings-card">
        card for {String((slotProps as { pluginId: string })?.pluginId)}
      </div>
    );
    pluginRegistry.forPlugin("plugin-a").registerComponent(OWNER_SLOT, Card);
    pluginRegistry.forPlugin("plugin-b").registerComponent(OWNER_SLOT, Card);

    // Viewing plugin-b's page: only plugin-b's component renders, with its id.
    render(
      <PluginSlot
        name={OWNER_SLOT}
        ownerPluginId="plugin-b"
        slotProps={{ pluginId: "plugin-b", status: "active" }}
      />,
    );

    const cards = screen.getAllByTestId("settings-card");
    expect(cards).toHaveLength(1);
    expect(cards[0]?.textContent).toBe("card for plugin-b");
  });

  it("resets the error boundary when the owner changes so a healthy plugin's card is not hidden", () => {
    // eslint-disable-next-line no-console -- expected error boundary log, assert + silence it
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});

    pluginRegistry.forPlugin("plugin-a").registerComponent(OWNER_SLOT, () => {
      throw new Error("boom");
    });
    pluginRegistry
      .forPlugin("plugin-b")
      .registerComponent(OWNER_SLOT, () => <div data-testid="slot-b">B</div>);

    // Same PluginSlot instance: plugin-a throws, then we "navigate" to plugin-b.
    const { rerender } = render(<PluginSlot name={OWNER_SLOT} ownerPluginId="plugin-a" />);
    rerender(<PluginSlot name={OWNER_SLOT} ownerPluginId="plugin-b" />);

    // plugin-b is healthy and must render, not be hidden behind plugin-a's error.
    expect(screen.getByTestId("slot-b")).not.toBeNull();
    consoleError.mockRestore();
  });
});

describe("PluginSlot registration identity", () => {
  it("isolates a throwing slot component so a sibling still renders", () => {
    // eslint-disable-next-line no-console -- expected error boundary log, assert + silence it
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});

    pluginRegistry.forPlugin("plugin-a").registerComponent(SLOT, () => {
      throw new Error("boom");
    });
    pluginRegistry
      .forPlugin("plugin-b")
      .registerComponent(SLOT, () => <div data-testid="slot-b">B</div>);

    render(<PluginSlot name={SLOT} />);

    expect(screen.getByTestId("slot-b")).not.toBeNull();
    expect(consoleError).toHaveBeenCalledWith(
      '[plugins] plugin "plugin-a" slot "task-sidebar" component threw',
      expect.any(Error),
      expect.any(String),
    );
    consoleError.mockRestore();
  });

  it("does not give a healthy plugin the removed plugin's error state", () => {
    // eslint-disable-next-line no-console -- expected error boundary log, assert + silence it
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});

    pluginRegistry.forPlugin("plugin-a").registerComponent(SLOT, () => {
      throw new Error("boom");
    });
    pluginRegistry
      .forPlugin("plugin-b")
      .registerComponent(SLOT, () => <div data-testid="slot-b">B</div>);

    render(<PluginSlot name={SLOT} />);

    act(() => {
      pluginRegistry.unregisterPlugin("plugin-a");
    });

    expect(screen.getByTestId("slot-b")).not.toBeNull();
    consoleError.mockRestore();
  });

  it("keeps healthy plugin component identity through live slot changes", () => {
    let nextMountId = 0;
    function StatefulSlot({ id }: { id: string }) {
      const [mountId] = useState(() => ++nextMountId);
      return <div data-testid={id}>{mountId}</div>;
    }

    pluginRegistry
      .forPlugin("plugin-a")
      .registerComponent(SLOT, () => <StatefulSlot id="slot-a" />);
    pluginRegistry
      .forPlugin("plugin-b")
      .registerComponent(SLOT, () => <StatefulSlot id="slot-b" />);

    render(<PluginSlot name={SLOT} />);

    expect(screen.getByTestId("slot-a").textContent).toBe("1");
    expect(screen.getByTestId("slot-b").textContent).toBe("2");

    act(() => {
      pluginRegistry
        .forPlugin("plugin-c")
        .registerComponent(SLOT, () => <StatefulSlot id="slot-c" />);
    });

    expect(screen.getByTestId("slot-a").textContent).toBe("1");
    expect(screen.getByTestId("slot-b").textContent).toBe("2");
    expect(screen.getByTestId("slot-c").textContent).toBe("3");

    act(() => {
      pluginRegistry.unregisterPlugin("plugin-a");
    });

    expect(screen.getByTestId("slot-b").textContent).toBe("2");
    expect(screen.getByTestId("slot-c").textContent).toBe("3");
  });
});

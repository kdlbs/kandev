import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import { PluginSlot } from "./plugin-slot";

const SLOT = "task-sidebar";

function cleanupPlugins(...pluginIds: string[]) {
  pluginIds.forEach((id) => pluginRegistry.unregisterPlugin(id));
}

describe("PluginSlot", () => {
  afterEach(() => {
    cleanup();
    cleanupPlugins("plugin-a", "plugin-b");
  });

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

  it("renders the plugin-settings slot and forwards slotProps.pluginId to gate per plugin", () => {
    // The "plugin-settings" slot renders on every plugin's detail page, so a
    // registered component early-returns unless slotProps.pluginId is its own id.
    const ProviderUsageCard = ({ slotProps }: { slotProps?: unknown }) => {
      const { pluginId } = (slotProps as { pluginId: string }) ?? { pluginId: "" };
      if (pluginId !== "provider-usage") return null;
      return <div data-testid="provider-usage-card">status card for {pluginId}</div>;
    };
    pluginRegistry.forPlugin("plugin-a").registerComponent("plugin-settings", ProviderUsageCard);

    // Viewing a different plugin's page: the card gates itself out.
    const { rerender } = render(
      <PluginSlot
        name="plugin-settings"
        slotProps={{ pluginId: "other-plugin", status: "active" }}
      />,
    );
    expect(screen.queryByTestId("provider-usage-card")).toBeNull();

    // Viewing its own plugin's page: the card renders with the forwarded id.
    rerender(
      <PluginSlot
        name="plugin-settings"
        slotProps={{ pluginId: "provider-usage", status: "active" }}
      />,
    );
    expect(screen.getByTestId("provider-usage-card").textContent).toBe(
      "status card for provider-usage",
    );
  });

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
    consoleError.mockRestore();
  });
});

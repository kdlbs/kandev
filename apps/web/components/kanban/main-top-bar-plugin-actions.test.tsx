import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import { MainTopBarPluginActions, type MainTopBarSlotProps } from "./main-top-bar-plugin-actions";

const SLOT = "main-top-bar";

describe("MainTopBarPluginActions", () => {
  afterEach(() => {
    cleanup();
    pluginRegistry.unregisterPlugin("plugin-a");
  });

  it("renders nothing when no plugin registered a main-top-bar component", () => {
    const { container } = render(
      <MainTopBarPluginActions workspaceId="w1" workspaceLabel="Acme" currentPage="kanban" />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("forwards the active workspace and current view as slotProps", () => {
    pluginRegistry.forPlugin("plugin-a").registerComponent(SLOT, ({ slotProps }) => {
      const ctx = slotProps as MainTopBarSlotProps;
      return (
        <div data-testid="plugin-app-bar">
          {`${ctx.workspaceId}|${ctx.workspaceLabel}|${ctx.currentPage}|${ctx.presentation}`}
        </div>
      );
    });

    render(<MainTopBarPluginActions workspaceId="w1" workspaceLabel="Acme" currentPage="tasks" />);

    expect(screen.getByTestId("plugin-app-bar").textContent).toBe("w1|Acme|tasks|desktop");
  });

  it("normalizes an absent workspace id to null", () => {
    pluginRegistry.forPlugin("plugin-a").registerComponent(SLOT, ({ slotProps }) => {
      const ctx = slotProps as MainTopBarSlotProps;
      return <div data-testid="plugin-app-bar">{String(ctx.workspaceId)}</div>;
    });

    render(<MainTopBarPluginActions currentPage="kanban" />);

    expect(screen.getByTestId("plugin-app-bar").textContent).toBe("null");
  });

  it("passes mobile presentation and applies the native icon-button contract", () => {
    pluginRegistry.forPlugin("plugin-a").registerComponent(SLOT, ({ slotProps }) => {
      const ctx = slotProps as MainTopBarSlotProps;
      return (
        <button data-slot="button" data-testid="plugin-button">
          <svg />
          {ctx.presentation}
        </button>
      );
    });

    render(<MainTopBarPluginActions currentPage="kanban" presentation="mobile" />);

    expect(screen.getByTestId("plugin-button").textContent).toBe("mobile");
    expect(screen.getByTestId("mobile-main-top-bar-plugin-actions").className).toContain(
      "[&_[data-slot=button]]:!size-8",
    );
  });
});

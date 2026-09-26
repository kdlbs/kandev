import * as React from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { PluginActionGroupProps, PluginActionProps } from "@kandev/plugin-sdk";
import * as pluginActionSurface from "./plugin-action-surface";
import { PluginSlot } from "./plugin-slot";
import { pluginRegistry } from "@/lib/plugins/registry";
import { createAppStore } from "@/lib/state/store";
import { buildHostApi } from "@/lib/plugins/host-api";

const PLUGIN_ID = "plugin-action-tests";
const SLOT = "chat-input-actions";
const ACTION_SURFACE = { surface: "composer" as const, presentation: "desktop" as const };
const STATEFUL_ACTION_TEST_ID = "stateful-action";
const PLUGIN_DISCLOSURE_TEST_ID = "plugin-disclosure";

function cleanupPlugin() {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
}

function makeAction(props: PluginActionProps) {
  const host = buildHostApi(PLUGIN_ID, createAppStore());
  const Action = host.ui.Action as React.ComponentType<PluginActionProps>;
  return React.createElement(Action, props);
}

function register(component: (props: { slotProps?: unknown }) => React.ReactNode) {
  pluginRegistry.forPlugin(PLUGIN_ID).registerComponent(SLOT, component);
}

function renderSlot() {
  return render(
    <TooltipProvider>
      <PluginSlot name={SLOT} actionSurface={ACTION_SURFACE} />
    </TooltipProvider>,
  );
}

afterEach(cleanupPlugin);

describe("host.ui.Action rendering", () => {
  it("renders a stable labelled host button and bounded decorative value", () => {
    const action = makeAction({
      label: "Current provider usage",
      text: "a very long provider value that must be clipped",
      badge: "1000+",
      "data-testid": "plugin-action",
    });
    register(() => action);

    renderSlot();

    const button = screen.getByTestId("plugin-action");
    expect(button.tagName).toBe("BUTTON");
    expect(button.getAttribute("type")).toBe("button");
    expect(button.getAttribute("aria-label")).toBe("Current provider usage");
    expect(
      button.querySelector('[data-slot="surface-action-text"]')?.classList.contains("truncate"),
    ).toBe(true);
    expect(
      button.querySelector('[data-slot="surface-action-badge"]')?.getAttribute("aria-hidden"),
    ).toBe("true");
  });

  it("shows the accessible label when a standard action has no other visible content", () => {
    const action = makeAction({
      label: "Refresh provider data",
      tooltip: "",
      "data-testid": "label-fallback-action",
    });
    register(() => action);

    renderSlot();

    expect(screen.getByTestId("label-fallback-action").textContent).toBe("Refresh provider data");
  });
});

describe("host.ui.Action interaction", () => {
  it("keeps busy stop controls enabled and forwards pointer capture events and refs", () => {
    const ref = React.createRef<HTMLButtonElement>();
    const pointerDown = vi.fn();
    const click = vi.fn();
    const action = makeAction({
      label: "Stop recording",
      icon: React.createElement("svg", { viewBox: "0 0 24 24" }),
      busy: true,
      ref,
      onPointerDown: (event) => {
        event.currentTarget.setPointerCapture(event.pointerId);
        pointerDown();
      },
      onClick: click,
      tooltip: "",
      "data-testid": "busy-action",
    });
    register(() => action);

    renderSlot();

    const button = screen.getByTestId("busy-action");
    expect(button.getAttribute("aria-busy")).toBe("true");
    expect((button as HTMLButtonElement).disabled).toBe(false);
    expect(ref.current).toBe(button);
    fireEvent.pointerDown(button, { pointerId: 7 });
    fireEvent.click(button);
    expect(pointerDown).toHaveBeenCalledOnce();
    expect(click).toHaveBeenCalledOnce();
  });

  it("keeps a disabled action's tooltip reachable through a focusable trigger", async () => {
    const action = makeAction({
      label: "Unavailable action",
      disabled: true,
      tooltip: "Connect an account to continue",
      "data-testid": "disabled-tooltip-action",
    });
    register(() => action);

    renderSlot();

    const button = screen.getByTestId("disabled-tooltip-action");
    const trigger = button.parentElement;
    expect(trigger).not.toBeNull();
    expect(trigger?.getAttribute("tabindex")).toBe("0");
    fireEvent.focus(trigger!);

    expect((await screen.findByRole("tooltip")).textContent).toBe("Connect an account to continue");
  });
});

describe("host.ui.Action prop contract", () => {
  it("ignores untyped style and button-shape props", () => {
    const host = buildHostApi(PLUGIN_ID, createAppStore());
    const Action = host.ui.Action as unknown as React.ComponentType<Record<string, unknown>>;
    const action = React.createElement(Action, {
      label: "Fixed host action",
      className: "h-1 w-1 p-0 bg-red-500",
      style: { height: 1, width: 1 },
      size: "sm",
      variant: "destructive",
      asChild: true,
      type: "submit",
      "data-testid": "fixed-action",
    });
    register(() => action);

    renderSlot();

    const button = screen.getByTestId("fixed-action");
    expect(button.getAttribute("type")).toBe("button");
    expect(button.getAttribute("style")).toBeNull();
    expect(button.className.split(/\s+/)).not.toContain("h-1");
    expect(button.className).not.toContain("bg-red-500");
    expect(button.getAttribute("data-slot")).toBe("surface-action");
  });

  it("keeps the accessible label without showing a touch tooltip", () => {
    const action = makeAction({
      label: "Open workspace tools",
      icon: React.createElement("svg", { viewBox: "0 0 24 24" }),
      "data-testid": "touch-action",
    });
    register(() => action);

    render(
      <TooltipProvider>
        <PluginSlot name={SLOT} actionSurface={{ surface: "topbar", presentation: "mobile" }} />
      </TooltipProvider>,
    );

    expect(screen.getByTestId("touch-action").getAttribute("aria-label")).toBe(
      "Open workspace tools",
    );
    expect(screen.queryByRole("tooltip")).toBeNull();
  });
});

describe("host.ui.Action state", () => {
  it("keeps plugin state when the host changes the action presentation", () => {
    const host = buildHostApi(PLUGIN_ID, createAppStore());
    const Action = host.ui.Action as React.ComponentType<PluginActionProps>;
    function StatefulAction() {
      const [count, setCount] = React.useState(0);
      return React.createElement(Action, {
        label: "Action count",
        text: String(count),
        tooltip: "",
        onClick: () => setCount((previous) => previous + 1),
        "data-testid": STATEFUL_ACTION_TEST_ID,
      });
    }
    register(StatefulAction);
    const { rerender } = render(
      <TooltipProvider>
        <PluginSlot name={SLOT} actionSurface={ACTION_SURFACE} />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByTestId(STATEFUL_ACTION_TEST_ID));
    expect(screen.getByTestId(STATEFUL_ACTION_TEST_ID).textContent).toBe("1");
    rerender(
      <TooltipProvider>
        <PluginSlot name={SLOT} actionSurface={{ surface: "composer", presentation: "mobile" }} />
      </TooltipProvider>,
    );

    expect(screen.getByTestId(STATEFUL_ACTION_TEST_ID).textContent).toBe("1");
    expect(screen.getByTestId(STATEFUL_ACTION_TEST_ID).getAttribute("data-presentation")).toBe(
      "mobile",
    );
  });
});

describe("host.ui.Action disclosure", () => {
  it("works as a cloneable popover trigger and preserves focus return", async () => {
    const host = buildHostApi(PLUGIN_ID, createAppStore());
    const Popover = host.ui.Popover as React.ComponentType<{ children?: React.ReactNode }>;
    const PopoverTrigger = host.ui.PopoverTrigger as React.ComponentType<{
      asChild?: boolean;
      children?: React.ReactNode;
    }>;
    const PopoverContent = host.ui.PopoverContent as React.ComponentType<
      React.ComponentProps<"div"> & { "data-testid"?: string }
    >;
    const Action = host.ui.Action as React.ComponentType<PluginActionProps>;
    function Disclosure() {
      return React.createElement(
        Popover,
        null,
        React.createElement(
          PopoverTrigger,
          { asChild: true },
          React.createElement(Action, {
            label: "Open plugin details",
            tooltip: "",
            "data-testid": "popover-trigger",
          }),
        ),
        React.createElement(
          PopoverContent,
          { "data-testid": PLUGIN_DISCLOSURE_TEST_ID },
          "Plugin detail",
        ),
      );
    }
    register(Disclosure);

    renderSlot();

    const trigger = screen.getByTestId("popover-trigger");
    trigger.focus();
    fireEvent.click(trigger);
    expect(await screen.findByTestId(PLUGIN_DISCLOSURE_TEST_ID)).toBeTruthy();
    fireEvent.keyDown(screen.getByTestId(PLUGIN_DISCLOSURE_TEST_ID), {
      key: "Escape",
      code: "Escape",
    });
    expect(screen.queryByTestId(PLUGIN_DISCLOSURE_TEST_ID)).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });
});

describe("host.ui.ActionGroup", () => {
  it("returns no wrapper for empty direct children and groups standard actions", () => {
    const host = buildHostApi(PLUGIN_ID, createAppStore());
    const ActionGroup = host.ui.ActionGroup as React.ComponentType<PluginActionGroupProps>;
    const Action = host.ui.Action as React.ComponentType<PluginActionProps>;
    const empty = React.createElement(ActionGroup, { children: null });
    function Group() {
      return React.createElement(
        ActionGroup,
        { label: "Provider actions" },
        React.createElement(Action, { label: "Refresh" }),
        React.createElement(Action, { label: "Open" }),
      );
    }
    register(() => React.createElement(React.Fragment, null, empty, React.createElement(Group)));

    renderSlot();

    expect(screen.getByRole("group", { name: "Provider actions" }).getAttribute("data-slot")).toBe(
      "surface-action-group",
    );
    expect(screen.getAllByRole("button")).toHaveLength(2);
    expect(document.querySelectorAll('[data-slot="surface-action-group"]')).toHaveLength(1);
  });

  it("keeps the plugin slot mounted when conditional actions become empty", () => {
    const host = buildHostApi(PLUGIN_ID, createAppStore());
    const ActionGroup = host.ui.ActionGroup as React.ComponentType<PluginActionGroupProps>;
    const Action = host.ui.Action as React.ComponentType<PluginActionProps>;
    const readSurface = vi.spyOn(pluginActionSurface, "usePluginActionSurface");
    function ConditionalGroup() {
      const [showAction, setShowAction] = React.useState(true);
      return (
        <>
          <button data-testid="toggle-conditional-action" onClick={() => setShowAction(false)}>
            Hide action
          </button>
          <ActionGroup label="Conditional actions">
            {showAction ? <Action label="Refresh" data-testid="conditional-action" /> : null}
          </ActionGroup>
        </>
      );
    }
    register(ConditionalGroup);

    renderSlot();
    const callsBeforeEmptyGroup = readSurface.mock.calls.length;
    expect(readSurface).toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("toggle-conditional-action"));

    expect(screen.getByTestId("toggle-conditional-action")).toBeTruthy();
    expect(screen.queryByTestId("conditional-action")).toBeNull();
    expect(screen.queryByRole("group", { name: "Conditional actions" })).toBeNull();
    expect(readSurface.mock.calls.length).toBeGreaterThan(callsBeforeEmptyGroup);
  });
});

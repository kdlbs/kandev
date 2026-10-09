import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState, type ComponentType } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { Component as PluginSDKComponent, PluginActionProps } from "@kandev/plugin-sdk";
import { createAppStore } from "@/lib/state/store";
import { PluginSlot } from "@/components/plugins/plugin-slot";
import { pluginRegistry } from "./registry";
import { buildHostApi } from "./host-api";
import {
  LegacyActionContribution,
  LegacyRawMetricButton,
} from "./__fixtures__/action-compatibility/legacy-controls";

const SLOT = "main-top-bar";
const ACTION_SURFACE = { surface: "topbar", presentation: "desktop" } as const;
const PLUGIN_ID = {
  legacy: "legacy-source-fixture",
  adopted: "adopted-fixture",
  null: "null-fixture",
  oldHost: "old-host-fixture",
  newHost: "new-host-fixture",
} as const;
const TEST_ID = {
  legacyHostButton: "legacy-host-button",
  legacyRawMetric: "legacy-raw-metric",
  legacyPreviewTrigger: "legacy-preview-trigger",
  legacyPreviewContent: "legacy-preview-content",
  legacyStatus: "legacy-status-contribution",
  adoptedAction: "adopted-action",
  independentAction: "independent-action",
  featureDetectedAction: "feature-detected-action",
  legacyFallbackAction: "legacy-fallback-action",
} as const;
const TRUE_ATTRIBUTE_VALUE = "true";
const PRESSED_ATTRIBUTE_NAME = "aria-pressed";
const PLUGIN_IDS = [PLUGIN_ID.legacy, PLUGIN_ID.adopted, PLUGIN_ID.null, PLUGIN_ID.oldHost];

function renderPluginSlot(ownerPluginId?: string) {
  return (
    <TooltipProvider>
      <PluginSlot name={SLOT} ownerPluginId={ownerPluginId} actionSurface={ACTION_SURFACE} />
    </TooltipProvider>
  );
}

afterEach(() => {
  cleanup();
  PLUGIN_IDS.forEach((pluginId) => pluginRegistry.unregisterPlugin(pluginId));
});

describe("source-attributed legacy action fixtures", () => {
  it("keeps old controls, context, state, and disclosure behavior beside a standard Action", () => {
    const host = buildHostApi(PLUGIN_ID.legacy, createAppStore());
    const hostButtonActivated = vi.fn();
    const Action = host.ui.Action as ComponentType<PluginActionProps>;

    pluginRegistry
      .forPlugin(PLUGIN_ID.legacy)
      .registerComponent(SLOT, ({ slotProps }) => (
        <LegacyActionContribution
          host={host}
          slotProps={slotProps}
          onHostButtonActivate={hostButtonActivated}
        />
      ));
    pluginRegistry.forPlugin(PLUGIN_ID.adopted).registerComponent(SLOT, function AdoptedAction() {
      const [pressed, setPressed] = useState(false);
      return (
        <Action
          label="Standard plugin action"
          pressed={pressed}
          data-testid={TEST_ID.adoptedAction}
          onClick={() => setPressed((previous) => !previous)}
        />
      );
    });

    render(
      <TooltipProvider>
        <PluginSlot
          name={SLOT}
          slotProps={{ placement: "right", presentation: "bar", activeTaskId: "task-a" }}
          actionSurface={ACTION_SURFACE}
        />
      </TooltipProvider>,
    );

    const hostButton = screen.getByTestId(TEST_ID.legacyHostButton);
    const rawMetric = screen.getByTestId(TEST_ID.legacyRawMetric);
    const previewTrigger = screen.getByTestId(TEST_ID.legacyPreviewTrigger);
    const adoptedAction = screen.getByTestId(TEST_ID.adoptedAction);
    expect(hostButton.className).toContain("h-7");
    expect(rawMetric.className).toContain("border-amber-600");
    expect(screen.getByTestId(TEST_ID.legacyStatus).textContent).toBe("right:bar:task-a");
    expect(screen.getAllByRole("button")).toHaveLength(4);

    fireEvent.click(hostButton);
    fireEvent.click(rawMetric);
    fireEvent.click(previewTrigger);
    fireEvent.click(adoptedAction);

    expect(hostButtonActivated).toHaveBeenCalledTimes(1);
    expect(rawMetric.getAttribute("data-activated")).toBe(TRUE_ATTRIBUTE_VALUE);
    expect(previewTrigger.getAttribute("aria-expanded")).toBe(TRUE_ATTRIBUTE_VALUE);
    expect(screen.getByTestId(TEST_ID.legacyPreviewContent).hasAttribute("hidden")).toBe(false);
    expect(adoptedAction.getAttribute(PRESSED_ATTRIBUTE_NAME)).toBe(TRUE_ATTRIBUTE_VALUE);
  });

  it("isolates owners through disable and re-enable, and keeps null contributions absent", () => {
    const host = buildHostApi(PLUGIN_ID.legacy, createAppStore());
    const Action = host.ui.Action as ComponentType<PluginActionProps>;
    const registerLegacy = () =>
      pluginRegistry
        .forPlugin(PLUGIN_ID.legacy)
        .registerComponent(SLOT, () => <LegacyRawMetricButton />);
    registerLegacy();
    pluginRegistry.forPlugin(PLUGIN_ID.adopted).registerComponent(SLOT, function StatefulAction() {
      const [pressed, setPressed] = useState(false);
      return (
        <Action
          label="Independent plugin action"
          pressed={pressed}
          data-testid={TEST_ID.independentAction}
          onClick={() => setPressed((previous) => !previous)}
        />
      );
    });

    const view = render(renderPluginSlot());
    fireEvent.click(screen.getByTestId(TEST_ID.legacyRawMetric));
    fireEvent.click(screen.getByTestId(TEST_ID.independentAction));
    expect(screen.getByTestId(TEST_ID.legacyRawMetric).getAttribute("data-activated")).toBe(
      TRUE_ATTRIBUTE_VALUE,
    );
    expect(screen.getByTestId(TEST_ID.independentAction).getAttribute(PRESSED_ATTRIBUTE_NAME)).toBe(
      TRUE_ATTRIBUTE_VALUE,
    );

    act(() => pluginRegistry.unregisterPlugin(PLUGIN_ID.legacy));
    view.rerender(renderPluginSlot());
    expect(screen.queryByTestId(TEST_ID.legacyRawMetric)).toBeNull();
    expect(screen.getByTestId(TEST_ID.independentAction).getAttribute(PRESSED_ATTRIBUTE_NAME)).toBe(
      TRUE_ATTRIBUTE_VALUE,
    );

    act(registerLegacy);
    expect(screen.getByTestId(TEST_ID.legacyRawMetric).getAttribute("data-activated")).toBe(
      "false",
    );
    expect(screen.getByTestId(TEST_ID.independentAction).getAttribute(PRESSED_ATTRIBUTE_NAME)).toBe(
      TRUE_ATTRIBUTE_VALUE,
    );

    pluginRegistry.forPlugin(PLUGIN_ID.null).registerComponent(SLOT, () => null);
    const { container } = render(renderPluginSlot(PLUGIN_ID.null));
    expect(container.innerHTML).toBe("");
  });
});

type CompatibleHost = {
  ui: {
    Button: unknown;
    Action?: unknown;
  };
};

function registerFeatureDetectedAction(
  host: CompatibleHost,
  register: (component: PluginSDKComponent<{ slotProps?: unknown }>) => void,
) {
  const Action = host.ui.Action as ComponentType<PluginActionProps> | undefined;
  const Button = host.ui.Button as ComponentType<{
    type: "button";
    "aria-label": string;
    "data-testid": string;
    className: string;
    children?: string;
  }>;
  const Component: PluginSDKComponent<{ slotProps?: unknown }> = () =>
    Action ? (
      <Action label="Feature-detected action" data-testid={TEST_ID.featureDetectedAction} />
    ) : (
      <Button
        type="button"
        aria-label="Legacy fallback action"
        data-testid={TEST_ID.legacyFallbackAction}
        className="h-7 w-7"
      >
        Legacy
      </Button>
    );
  register(Component);
}

describe("additive Action host compatibility", () => {
  it("selects one contribution on new and old hosts without duplicate registration", () => {
    const currentHost = buildHostApi(PLUGIN_ID.newHost, createAppStore());
    const newHost: CompatibleHost = {
      ui: { Button: currentHost.ui.Button, Action: currentHost.ui.Action },
    };
    const oldHost: CompatibleHost = { ui: { Button: currentHost.ui.Button } };
    const newRegistry = pluginRegistry.forPlugin(PLUGIN_ID.adopted);
    const oldRegistry = pluginRegistry.forPlugin(PLUGIN_ID.oldHost);
    const registerNew = vi.fn((component: PluginSDKComponent<{ slotProps?: unknown }>) =>
      newRegistry.registerComponent(SLOT, component),
    );
    const registerOld = vi.fn((component: PluginSDKComponent<{ slotProps?: unknown }>) =>
      oldRegistry.registerComponent(SLOT, component),
    );

    registerFeatureDetectedAction(newHost, registerNew);
    registerFeatureDetectedAction(oldHost, registerOld);

    expect(registerNew).toHaveBeenCalledTimes(1);
    expect(registerOld).toHaveBeenCalledTimes(1);
    expect(
      pluginRegistry
        .getSlotRegistrations(SLOT)
        .filter(({ pluginId }) => pluginId === PLUGIN_ID.adopted),
    ).toHaveLength(1);
    expect(
      pluginRegistry
        .getSlotRegistrations(SLOT)
        .filter(({ pluginId }) => pluginId === PLUGIN_ID.oldHost),
    ).toHaveLength(1);

    render(renderPluginSlot());
    expect(screen.getAllByTestId(TEST_ID.featureDetectedAction)).toHaveLength(1);
    expect(screen.getAllByTestId(TEST_ID.legacyFallbackAction)).toHaveLength(1);
    expect(screen.getByTestId(TEST_ID.featureDetectedAction).getAttribute("data-slot")).toBe(
      "surface-action",
    );
    expect(screen.getByTestId(TEST_ID.legacyFallbackAction).getAttribute("data-slot")).toBe(
      "button",
    );
  });
});

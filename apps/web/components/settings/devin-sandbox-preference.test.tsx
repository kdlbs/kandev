import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { Mock } from "vitest";
import { afterEach, describe, expect, it, vi } from "vitest";

import { DevinSandboxPreferenceField } from "@/components/settings/devin-sandbox-preference";
import {
  DEVIN_AUTONOMOUS_MODE,
  DEVIN_SANDBOX_ENV_KEY,
  DEVIN_SMART_MODE,
} from "@/lib/devin-profile-settings";

afterEach(() => {
  cleanup();
});

const onChange = () => vi.fn();

function renderField(overrides: {
  agentName?: string;
  mode?: string;
  envVars?: { key: string; value?: string; secret_id?: string }[];
  baselineEnvVars?: { key: string; value?: string; secret_id?: string }[];
  baselineMode?: string;
  smartAvailable?: boolean;
  discoveryReady?: boolean;
  handler?: Mock<
    (patch: {
      mode: string;
      env_vars: { key: string; value?: string; secret_id?: string }[];
    }) => void
  >;
}) {
  const handler =
    overrides.handler ??
    vi.fn<
      (patch: {
        mode: string;
        env_vars: { key: string; value?: string; secret_id?: string }[];
      }) => void
    >();
  render(
    <DevinSandboxPreferenceField
      agentName={overrides.agentName ?? "devin-acp"}
      profile={{ mode: overrides.mode ?? DEVIN_SMART_MODE, env_vars: overrides.envVars ?? [] }}
      baselineProfile={{
        mode: overrides.baselineMode ?? overrides.mode ?? DEVIN_SMART_MODE,
        env_vars: overrides.baselineEnvVars ?? overrides.envVars ?? [],
      }}
      smartAvailable={overrides.smartAvailable ?? true}
      discoveryReady={overrides.discoveryReady ?? true}
      onChange={handler}
    />,
  );
  return handler;
}

async function choose(label: string) {
  fireEvent.click(screen.getByRole("combobox", { name: /Devin sandbox/ }));
  fireEvent.click(await screen.findByRole("option", { name: label }));
}

describe("DevinSandboxPreferenceField", () => {
  it("renders only for the Devin agent", () => {
    renderField({ agentName: "codex-acp" });
    expect(screen.queryByTestId("devin-sandbox-preference")).toBeNull();
  });

  it("shows inherited, disabled, and enabled states accurately", () => {
    renderField({});
    expect(screen.getByRole("combobox", { name: /Devin sandbox/ }).textContent).toContain(
      "Use CLI settings",
    );
    cleanup();
    renderField({ envVars: [{ key: DEVIN_SANDBOX_ENV_KEY, value: "false" }] });
    expect(screen.getByRole("combobox", { name: /Devin sandbox/ }).textContent).toContain(
      "Disabled",
    );
    cleanup();
    renderField({ envVars: [{ key: DEVIN_SANDBOX_ENV_KEY, value: "true" }] });
    expect(screen.getByRole("combobox", { name: /Devin sandbox/ }).textContent).toContain(
      "Enabled",
    );
    expect(screen.getByTestId("devin-sandbox-autonomous").textContent).toContain("Autonomous");
  });

  it("enabling selects Autonomous with the override", async () => {
    const handler = renderField({ mode: DEVIN_SMART_MODE });
    await choose("Enabled");
    expect(handler).toHaveBeenCalledWith({
      mode: DEVIN_AUTONOMOUS_MODE,
      env_vars: [{ key: DEVIN_SANDBOX_ENV_KEY, value: "true" }],
      devin_sandbox_inherited: false,
    });
  });

  it("disabling an Autonomous profile returns to Smart", async () => {
    const handler = renderField({
      mode: DEVIN_AUTONOMOUS_MODE,
      envVars: [{ key: DEVIN_SANDBOX_ENV_KEY, value: "true" }],
    });
    await choose("Disabled");
    expect(handler).toHaveBeenCalledWith({
      mode: DEVIN_SMART_MODE,
      env_vars: [{ key: DEVIN_SANDBOX_ENV_KEY, value: "false" }],
      devin_sandbox_inherited: false,
    });
  });

  it("preserves an explicit mode when inheriting the CLI setting", async () => {
    const handler = renderField({
      mode: DEVIN_AUTONOMOUS_MODE,
      envVars: [{ key: DEVIN_SANDBOX_ENV_KEY, value: "true" }],
    });
    await choose("Use CLI settings");
    expect(handler).toHaveBeenCalledWith({
      mode: DEVIN_AUTONOMOUS_MODE,
      env_vars: [],
      devin_sandbox_inherited: true,
    });
  });

  it("keeps a custom value visible until replaced", () => {
    renderField({ envVars: [{ key: DEVIN_SANDBOX_ENV_KEY, value: "custom" }] });
    expect(screen.getByTestId("devin-sandbox-custom").textContent).toContain("custom sandbox");
  });

  it("announces an unavailable Smart mode", () => {
    renderField({ mode: DEVIN_SMART_MODE, smartAvailable: false });
    expect(screen.getByTestId("devin-smart-unavailable").getAttribute("role")).toBe("alert");
    expect(screen.getByTestId("devin-smart-unavailable").textContent).toContain(
      "Smart is unavailable",
    );
  });

  it("exposes a touch-sized selector", () => {
    renderField({});
    const trigger = screen.getByRole("combobox", { name: /Devin sandbox/ });
    expect(trigger.className).toContain("h-7");
    expect(trigger.className).toContain("max-md:h-11");
    expect(trigger.className).toContain("[@media(pointer:coarse)]:h-11");
  });

  it("keeps the onChange callback unused until the user selects", () => {
    const handler = onChange();
    renderField({ handler });
    expect(handler).not.toHaveBeenCalled();
  });
});

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { PluginConfigField } from "@/lib/plugins/config-schema";
import { PluginConfigForm } from "./plugin-config-form";

const fields: PluginConfigField[] = [
  {
    name: "greeting",
    label: "Greeting",
    type: "string",
    required: false,
    secret: false,
  },
  {
    name: "enabled",
    label: "Enabled",
    type: "boolean",
    required: false,
    secret: false,
  },
];

afterEach(cleanup);

describe("PluginConfigForm", () => {
  it("marks only changed controls as dirty", () => {
    render(
      <PluginConfigForm
        fields={fields}
        values={{ greeting: "changed", enabled: false }}
        initialValues={{ greeting: "saved", enabled: false }}
        disabled={false}
        onChange={vi.fn()}
      />,
    );

    expect(screen.getByLabelText("Greeting").getAttribute("data-settings-dirty")).toBe("true");
    expect(screen.getByLabelText("Enabled").getAttribute("data-settings-dirty")).toBe("false");
  });
});

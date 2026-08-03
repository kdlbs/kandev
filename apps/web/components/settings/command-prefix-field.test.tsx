import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CommandPrefixField } from "./command-prefix-field";
import type { ProfileFormData } from "./profile-form-fields";

afterEach(cleanup);
const DIRTY_ATTRIBUTE = "data-settings-dirty";
const PREFIX = "greywall --";

function formData(overrides: Partial<ProfileFormData> = {}): ProfileFormData {
  return {
    name: "Profile",
    model: "mock-fast",
    mode: "",
    cli_passthrough: false,
    cli_flags: [],
    command_prefix: "",
    ...overrides,
  } as ProfileFormData;
}

function dirtyState(): string | null {
  return screen
    .getByTestId("command-prefix-input")
    .closest(`[${DIRTY_ATTRIBUTE}]`)
    ?.getAttribute(DIRTY_ATTRIBUTE) as string | null;
}

describe("CommandPrefixField", () => {
  it("is clean when the value matches the baseline", () => {
    render(
      <CommandPrefixField
        profile={formData({ command_prefix: PREFIX })}
        baselineProfile={formData({ command_prefix: PREFIX })}
        onChange={vi.fn()}
      />,
    );

    expect(dirtyState()).toBe("false");
  });

  it("marks the field dirty when a prefix is set that was previously empty", () => {
    render(
      <CommandPrefixField
        profile={formData({ command_prefix: PREFIX })}
        baselineProfile={formData({ command_prefix: "" })}
        onChange={vi.fn()}
      />,
    );

    expect(dirtyState()).toBe("true");
  });

  it("marks the field dirty when a previously-saved prefix is cleared", () => {
    render(
      <CommandPrefixField
        profile={formData({ command_prefix: "" })}
        baselineProfile={formData({ command_prefix: PREFIX })}
        onChange={vi.fn()}
      />,
    );

    expect(dirtyState()).toBe("true");
  });

  it("reports the cleared value through onChange", () => {
    const onChange = vi.fn();
    render(
      <CommandPrefixField
        profile={formData({ command_prefix: PREFIX })}
        baselineProfile={formData({ command_prefix: PREFIX })}
        onChange={onChange}
      />,
    );

    fireEvent.change(screen.getByTestId("command-prefix-input"), { target: { value: "" } });
    expect(onChange).toHaveBeenCalledWith({ command_prefix: "" });
  });

  // The help text is a <Trans>: its <0> index addresses the JSX children
  // positionally, so a prettier reflow can silently reassemble the sentence
  // into fragments without failing anything.
  it("renders the help text as one sentence with the example launcher intact", () => {
    render(
      <CommandPrefixField profile={formData()} baselineProfile={formData()} onChange={vi.fn()} />,
    );

    const help = screen.getByText(
      (_content, element) =>
        element?.tagName === "P" &&
        element.textContent ===
          "Tokens prepended to the agent launch command, so it runs under a sandbox launcher " +
            "(e.g. greywall --). The value is shell-tokenised. Leave empty to run the agent " +
            "directly. Applies to ACP sessions only \u2014 it has no effect when the profile " +
            "uses TUI passthrough.",
    );
    expect(help.querySelector("code")?.textContent).toBe(PREFIX);
  });
});

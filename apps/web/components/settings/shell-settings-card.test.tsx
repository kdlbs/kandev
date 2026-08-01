import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ShellSettingsCard } from "./shell-settings-card";

const shellOptions = [
  { value: "/bin/bash", label: "bash" },
  { value: "/bin/zsh", label: "zsh" },
];

afterEach(cleanup);

describe("ShellSettingsCard", () => {
  it("renders its copy through the catalog", () => {
    render(
      <ShellSettingsCard
        preferredShell=""
        onPreferredShellChange={vi.fn()}
        shellLoaded
        shellOptions={shellOptions}
      />,
    );

    expect(screen.getByText("Shell")).toBeTruthy();
    expect(screen.getByText("Pick the default shell for task sessions")).toBeTruthy();
    expect(screen.getByText("Preferred Shell")).toBeTruthy();
    expect(
      screen.getByText(
        "New task sessions will use this shell. Existing sessions keep their current shell.",
      ),
    ).toBeTruthy();
  });

  it("keeps the example shell path untranslated in the custom input", () => {
    // A shell not in `shellOptions` resolves to the custom branch, which is the
    // only place the placeholder renders. The placeholder is an example path —
    // data, not copy — so it must stay verbatim in every locale.
    render(
      <ShellSettingsCard
        preferredShell="/opt/homebrew/bin/fish"
        onPreferredShellChange={vi.fn()}
        shellLoaded
        shellOptions={shellOptions}
      />,
    );

    const customInput = screen.getByPlaceholderText("/bin/zsh");
    expect((customInput as HTMLInputElement).value).toBe("/opt/homebrew/bin/fish");
    expect(
      screen.getByText("Enter a shell path or command available in the agent environment."),
    ).toBeTruthy();
  });
});

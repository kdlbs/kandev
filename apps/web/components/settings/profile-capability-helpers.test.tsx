import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { CommandsButton } from "./profile-capability-helpers";
import type { CommandEntry } from "@/lib/types/http";

afterEach(cleanup);

const COMMANDS: CommandEntry[] = [
  { name: "init", description: "Bootstrap the repo" },
  { name: "review", description: "" },
];

describe("CommandsButton", () => {
  it("renders nothing when the agent advertises no commands", () => {
    const { container } = render(<CommandsButton commands={[]} />);
    expect(container.firstChild).toBeNull();
  });

  it("counts the advertised commands on the trigger", () => {
    render(<CommandsButton commands={COMMANDS} />);
    expect(screen.getByTestId("profile-commands-button").textContent).toContain(
      "Available commands (2)",
    );
  });

  // The dialog description is a <Trans>: its <0> index addresses the JSX
  // children positionally, so a prettier reflow can silently reassemble the
  // sentence into fragments without failing anything. The `/init` example is a
  // command the user types verbatim and is interpolated as a value.
  it("renders the dialog description as one sentence with the example intact", () => {
    render(<CommandsButton commands={COMMANDS} />);
    fireEvent.click(screen.getByTestId("profile-commands-button"));

    const description = screen.getByText(
      (_content, element) =>
        element?.textContent === "Type these during a session chat to invoke them - e.g. /init." &&
        Boolean(element.querySelector("code")),
    );
    expect(description.querySelector("code")?.textContent).toBe("/init");
  });
});

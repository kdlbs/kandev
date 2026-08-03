import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { AddTUIAgentDialog } from "./add-tui-agent-dialog";

afterEach(cleanup);

function renderDialog(onSubmit = vi.fn()) {
  return render(<AddTUIAgentDialog open onOpenChange={vi.fn()} onSubmit={onSubmit} />);
}

describe("AddTUIAgentDialog", () => {
  // The command help is a <Trans>: its <0> index addresses the JSX children
  // positionally, so a prettier reflow can silently reassemble the sentence
  // into fragments without failing anything.
  it("renders the command help as one sentence with the {{model}} token intact", () => {
    renderDialog();

    const hint = screen.getByText(
      (_content, element) =>
        element?.tagName === "P" &&
        element.textContent ===
          "Binary name looked up on PATH. Use {{model}} to insert the model value.",
    );
    expect(hint.querySelector("code")?.textContent).toBe("{{model}}");
  });

  // `{{model}}` is a substitution token the user types verbatim. It is passed
  // through `t()` as an interpolation *value*, so i18next must not treat it as
  // a placeholder of its own and blank it out.
  it("keeps the {{model}} token in the command placeholder", () => {
    renderDialog();

    expect(screen.getByLabelText("Command").getAttribute("placeholder")).toBe(
      "e.g. superclaude --yolo --model {{model}}",
    );
  });

  it("rejects an empty display name without calling onSubmit", () => {
    const onSubmit = vi.fn();
    renderDialog(onSubmit);

    fireEvent.click(screen.getByText("Create"));

    expect(screen.getByText("Display name is required")).toBeTruthy();
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("rejects an empty command without calling onSubmit", () => {
    const onSubmit = vi.fn();
    renderDialog(onSubmit);

    fireEvent.change(screen.getByLabelText("Display Name"), { target: { value: "superclaude" } });
    fireEvent.click(screen.getByText("Create"));

    expect(screen.getByText("Command is required")).toBeTruthy();
    expect(onSubmit).not.toHaveBeenCalled();
  });

  // A non-Error rejection has no `.message`, so the handler falls back to the
  // catalog string. Nothing else exercises that branch.
  it("surfaces the translated fallback when onSubmit rejects with a non-Error", async () => {
    const onSubmit = vi.fn().mockRejectedValue("boom");
    renderDialog(onSubmit);

    fireEvent.change(screen.getByLabelText("Display Name"), { target: { value: "superclaude" } });
    fireEvent.change(screen.getByLabelText("Command"), { target: { value: "superclaude" } });
    fireEvent.click(screen.getByText("Create"));

    await waitFor(() => expect(screen.getByText("Failed to create agent")).toBeTruthy());
    expect(onSubmit).toHaveBeenCalledOnce();
  });
});

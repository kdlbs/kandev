import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { ApiError, INTERIM_SETTINGS_INTERLOCK_ERROR_CODE } from "@/lib/api/client";
import { AddTUIAgentDialog } from "./add-tui-agent-dialog";

const DISPLAY_NAME_LABEL = "Display Name";
const COMMAND_LABEL = "Command";

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

    expect(screen.getByLabelText(COMMAND_LABEL).getAttribute("placeholder")).toBe(
      "e.g. superagent --yolo --model {{model}}",
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

    fireEvent.change(screen.getByLabelText(DISPLAY_NAME_LABEL), {
      target: { value: "superagent" },
    });
    fireEvent.click(screen.getByText("Create"));

    expect(screen.getByText("Command is required")).toBeTruthy();
    expect(onSubmit).not.toHaveBeenCalled();
  });

  // A non-Error rejection has no `.message`, so the handler falls back to the
  // catalog string. Nothing else exercises that branch.
  it("surfaces the translated fallback when onSubmit rejects with a non-Error", async () => {
    const onSubmit = vi.fn().mockRejectedValue("boom");
    renderDialog(onSubmit);

    fireEvent.change(screen.getByLabelText(DISPLAY_NAME_LABEL), {
      target: { value: "superagent" },
    });
    fireEvent.change(screen.getByLabelText(COMMAND_LABEL), { target: { value: "superagent" } });
    fireEvent.click(screen.getByText("Create"));

    await waitFor(() => expect(screen.getByText("Failed to create agent")).toBeTruthy());
    expect(onSubmit).toHaveBeenCalledOnce();
  });

  // A custom agent that speaks ACP has to be created as one: the protocol is
  // part of the stored definition and decides whether kandev drives the command
  // over ACP or launches it in a terminal.
  it("submits the ACP protocol when it is selected", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    renderDialog(onSubmit);

    fireEvent.click(screen.getByTestId("agent-protocol-select"));
    fireEvent.click(screen.getByRole("option", { name: /ACP/ }));
    fireEvent.change(screen.getByLabelText(DISPLAY_NAME_LABEL), {
      target: { value: "My Agent" },
    });
    fireEvent.change(screen.getByLabelText(COMMAND_LABEL), {
      target: { value: "superagent --acp" },
    });
    fireEvent.click(screen.getByText("Create"));

    await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
    expect(onSubmit.mock.calls[0][0]).toMatchObject({
      display_name: "My Agent",
      command: "superagent --acp",
      protocol: "acp",
    });
  });

  it("defaults to the terminal protocol", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    renderDialog(onSubmit);

    fireEvent.change(screen.getByLabelText(DISPLAY_NAME_LABEL), {
      target: { value: "superagent" },
    });
    fireEvent.change(screen.getByLabelText(COMMAND_LABEL), { target: { value: "superagent" } });
    fireEvent.click(screen.getByText("Create"));

    await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce());
    expect(onSubmit.mock.calls[0][0].protocol).toBeUndefined();
  });

  // The passthrough MCP strategies write a config file for the wrapped CLI.
  // An ACP agent gets its servers in session/new, so the picker would offer a
  // choice the backend rejects.
  it("hides the MCP strategy picker for the ACP protocol", () => {
    renderDialog();

    expect(screen.queryByTestId("mcp-strategy-select")).not.toBeNull();

    fireEvent.click(screen.getByTestId("agent-protocol-select"));
    fireEvent.click(screen.getByRole("option", { name: /ACP/ }));

    expect(screen.queryByTestId("mcp-strategy-select")).toBeNull();
  });

  it("closes without exposing a handled stale-page error", async () => {
    const staleError = new ApiError("interim settings interlock required", 403, {
      error_code: INTERIM_SETTINGS_INTERLOCK_ERROR_CODE,
    });
    staleError.handled = true;
    const onSubmit = vi.fn().mockRejectedValue(staleError);
    const onOpenChange = vi.fn();
    render(<AddTUIAgentDialog open onOpenChange={onOpenChange} onSubmit={onSubmit} />);

    fireEvent.change(screen.getByLabelText(DISPLAY_NAME_LABEL), {
      target: { value: "superagent" },
    });
    fireEvent.change(screen.getByLabelText(COMMAND_LABEL), { target: { value: "superagent" } });
    fireEvent.click(screen.getByText("Create"));

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(screen.queryByText(staleError.message)).toBeNull();
    expect(onSubmit).toHaveBeenCalledOnce();
  });
});

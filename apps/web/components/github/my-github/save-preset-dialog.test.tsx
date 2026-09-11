import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SavePresetDialog } from "./save-preset-dialog";
afterEach(cleanup);

function renderDialog(onSave: (label: string, repo: string) => Promise<boolean>) {
  const onOpenChange = vi.fn();
  render(
    <SavePresetDialog
      open
      onOpenChange={onOpenChange}
      kind="pr"
      customQuery="is:open"
      repoFilter="org/repo"
      repoOptions={["org/repo"]}
      suggestedLabel="My review queue"
      onSave={onSave}
    />,
  );
  return onOpenChange;
}

// @covers AC-INTEGRATIONS-GITHUB-MOBILE-001.1
describe("SavePresetDialog persistence", () => {
  it("keeps the entered name after failure and closes only after a successful retry", async () => {
    const save = vi.fn().mockResolvedValueOnce(false).mockResolvedValueOnce(true);
    const onOpenChange = renderDialog(save);
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "New name" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
    expect(onOpenChange).not.toHaveBeenCalled();
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe("New name");
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Save" }).hasAttribute("disabled")).toBe(false),
    );
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(save).toHaveBeenLastCalledWith("New name", "org/repo");
  });

  it("prevents duplicate submission and dismissal while persistence is pending", async () => {
    let finish!: (success: boolean) => void;
    const save = vi.fn(
      () =>
        new Promise<boolean>((resolve) => {
          finish = resolve;
        }),
    );
    const onOpenChange = renderDialog(save);
    const submit = screen.getByRole("button", { name: "Save" });
    fireEvent.click(submit);
    fireEvent.click(submit);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(save).toHaveBeenCalledTimes(1);
    expect(onOpenChange).not.toHaveBeenCalled();
    await act(async () => finish(true));
    expect(onOpenChange).toHaveBeenCalledExactlyOnceWith(false);
  });
});

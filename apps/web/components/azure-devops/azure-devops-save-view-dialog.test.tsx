import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AzureDevOpsSaveViewDialog } from "./azure-devops-save-view-dialog";

afterEach(cleanup);

describe("AzureDevOpsSaveViewDialog", () => {
  it("closes only after a saved view is persisted", async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    const onOpenChange = vi.fn();
    render(
      <AzureDevOpsSaveViewDialog
        open
        kind="work_item"
        onOpenChange={onOpenChange}
        onSave={onSave}
      />,
    );

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "My work" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onSave).toHaveBeenCalledWith("My work"));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("stays open and reports a persistence failure", async () => {
    const onSave = vi.fn().mockRejectedValue(new Error("unavailable"));
    const onOpenChange = vi.fn();
    render(
      <AzureDevOpsSaveViewDialog
        open
        kind="pull_request"
        onOpenChange={onOpenChange}
        onSave={onSave}
      />,
    );

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Review queue" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect((await screen.findByRole("alert")).textContent).toContain("Could not save this view");
    expect(onOpenChange).not.toHaveBeenCalled();
  });
});

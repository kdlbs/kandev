import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ResetWatchDialog } from "./reset-watch-dialog";

afterEach(cleanup);

describe("ResetWatchDialog", () => {
  it("blocks reset in strict mode after preview failure and lets the user retry", async () => {
    const preview = vi
      .fn()
      .mockRejectedValueOnce(new Error("preview unavailable"))
      .mockResolvedValueOnce({ taskCount: 3 });
    render(
      <ResetWatchDialog
        open
        onOpenChange={vi.fn()}
        integrationLabel="GitLab watch"
        requirePreviewSuccess
        previewLoader={preview}
        onConfirm={vi.fn()}
      />,
    );

    expect(await screen.findByText(/could not load the affected task count/i)).toBeTruthy();
    expect((screen.getByRole("button", { name: "Reset" }) as HTMLButtonElement).disabled).toBe(
      true,
    );
    fireEvent.click(screen.getByRole("button", { name: "Retry preview" }));
    await waitFor(() => expect(screen.getByText(/delete 3 tasks/i)).toBeTruthy());
    expect((screen.getByRole("button", { name: "Reset" }) as HTMLButtonElement).disabled).toBe(
      false,
    );
  });

  it("keeps the legacy fallback flow when strict preview gating is omitted", async () => {
    const confirm = vi.fn().mockResolvedValue(undefined);
    render(
      <ResetWatchDialog
        open
        onOpenChange={vi.fn()}
        integrationLabel="GitHub watch"
        previewLoader={vi.fn().mockRejectedValue(new Error("preview unavailable"))}
        onConfirm={confirm}
      />,
    );

    expect(await screen.findByText(/delete every task previously created/i)).toBeTruthy();
    const reset = screen.getByRole("button", { name: "Reset" }) as HTMLButtonElement;
    expect(reset.disabled).toBe(false);
    expect(screen.queryByRole("button", { name: "Retry preview" })).toBeNull();
    fireEvent.click(reset);
    await waitFor(() => expect(confirm).toHaveBeenCalledTimes(1));
  });

  it("keeps the existing successful preview and confirm flow", async () => {
    const confirm = vi.fn().mockResolvedValue(undefined);
    render(
      <ResetWatchDialog
        open
        onOpenChange={vi.fn()}
        integrationLabel="GitHub watch"
        previewLoader={vi.fn().mockResolvedValue({ taskCount: 1 })}
        onConfirm={confirm}
      />,
    );
    expect(await screen.findByText(/delete 1 task/i)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    await waitFor(() => expect(confirm).toHaveBeenCalledTimes(1));
  });
});

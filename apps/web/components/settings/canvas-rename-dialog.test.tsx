import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Canvas } from "@/lib/api/domains/canvas-api";

const renameCanvas = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/canvas-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/canvas-api")>()),
  renameCanvas,
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));
vi.mock("@kandev/ui/dialog", () => ({
  Dialog: ({ children, open }: { children: ReactNode; open: boolean }) =>
    open ? <>{children}</> : null,
  DialogContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogFooter: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogHeader: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: ReactNode }) => <h2>{children}</h2>,
}));
import { CanvasRenameDialog } from "./canvas-rename-dialog";

const canvas = { id: "canvas-1", title: "Old title" } as Canvas;
const CANVAS_NAME_LABEL = "canvases:canvasName";
afterEach(() => {
  cleanup();
  renameCanvas.mockReset();
});

describe("CanvasRenameDialog", () => {
  it("trims and saves the title", async () => {
    renameCanvas.mockResolvedValue({ ...canvas, title: "New title" });
    const onOpenChange = vi.fn();
    const onRenamed = vi.fn();
    render(
      <CanvasRenameDialog canvas={canvas} open onOpenChange={onOpenChange} onRenamed={onRenamed} />,
    );
    fireEvent.change(screen.getByLabelText(CANVAS_NAME_LABEL), {
      target: { value: "  New title  " },
    });
    fireEvent.click(screen.getByRole("button", { name: "common:save" }));
    await waitFor(() => expect(renameCanvas).toHaveBeenCalledWith("canvas-1", "New title"));
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(onRenamed).toHaveBeenCalled();
  });

  it("retains the draft after failure and makes no request on cancel", async () => {
    renameCanvas.mockRejectedValue(new Error("offline"));
    const onOpenChange = vi.fn();
    render(
      <CanvasRenameDialog canvas={canvas} open onOpenChange={onOpenChange} onRenamed={vi.fn()} />,
    );
    fireEvent.change(screen.getByLabelText(CANVAS_NAME_LABEL), { target: { value: "Draft" } });
    fireEvent.click(screen.getByRole("button", { name: "common:save" }));
    await screen.findByRole("alert");
    expect((screen.getByLabelText(CANVAS_NAME_LABEL) as HTMLInputElement).value).toBe("Draft");
    fireEvent.click(screen.getByRole("button", { name: "common:cancel" }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(renameCanvas).toHaveBeenCalledTimes(1);
  });

  it("accepts a 200-code-point title with astral characters", async () => {
    renameCanvas.mockResolvedValue(canvas);
    render(<CanvasRenameDialog canvas={canvas} open onOpenChange={vi.fn()} onRenamed={vi.fn()} />);
    const title = "😀".repeat(200);
    fireEvent.change(screen.getByLabelText(CANVAS_NAME_LABEL), { target: { value: title } });
    fireEvent.click(screen.getByRole("button", { name: "common:save" }));
    await waitFor(() => expect(renameCanvas).toHaveBeenCalledWith("canvas-1", title));
  });
});

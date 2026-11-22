import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { ImageContextItem } from "@/lib/types/context";
import { ImageItem } from "./image-item";

afterEach(cleanup);

// @covers AC-AGENTS-SHARED-IMAGE-INSPECTION-001.3, AC-AGENTS-SHARED-IMAGE-INSPECTION-001.4
it("resets the composer image inspection and retains its attachment", async () => {
  const remove = vi.fn();
  const delivery = vi.fn();
  const item: ImageContextItem = {
    id: "image",
    kind: "image",
    label: "Screenshot",
    onRemove: remove,
    onDeliveryModeChange: delivery,
    attachment: {
      id: "image",
      mimeType: "image/png",
      fileName: "shot.png",
      size: 10,
      isImage: true,
      deliveryMode: "prompt",
      preview: "blob:first",
    },
  };
  const view = render(
    <div data-testid="composer-context-row">
      <ImageItem item={item} />
    </div>,
  );
  expect(screen.getByTestId("context-chip-remove").parentElement?.parentElement).toBe(
    screen.getByTestId("composer-context-row"),
  );
  const opener = screen.getByRole("button", { name: "Screenshot" });
  opener.focus();
  await act(async () => fireEvent.click(opener));
  let dialog = screen.getByRole("dialog");
  await act(async () =>
    fireEvent.click(within(dialog).getByRole("button", { name: "Actual size" })),
  );
  view.rerender(
    <div data-testid="composer-context-row">
      <ImageItem item={{ ...item, attachment: { ...item.attachment, preview: "blob:next" } }} />
    </div>,
  );
  dialog = screen.getByRole("dialog");
  expect(within(dialog).getByRole("img").getAttribute("src")).toBe("blob:next");
  expect(within(dialog).getByRole("button", { name: "Fit" }).getAttribute("aria-pressed")).toBe(
    "true",
  );
  await act(async () =>
    fireEvent.click(within(dialog).getByRole("button", { name: "Actual size" })),
  );
  await act(async () => fireEvent.click(within(dialog).getByRole("button", { name: "Close" })));
  await waitFor(() => expect(document.activeElement).toBe(opener));
  await act(async () => fireEvent.click(opener));
  expect(
    within(screen.getByRole("dialog"))
      .getByRole("button", { name: "Fit" })
      .getAttribute("aria-pressed"),
  ).toBe("true");
  expect(remove).not.toHaveBeenCalled();
  expect(delivery).not.toHaveBeenCalled();
});

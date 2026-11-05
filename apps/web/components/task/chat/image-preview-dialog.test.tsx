import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { ImagePreviewDialog } from "./image-preview-dialog";

const SOURCE = "data:image/png;base64,aW1hZ2U=";
const PRESSED = "aria-pressed";
afterEach(cleanup);

it("opens and closes an attachment image with its original source and label", async () => {
  render(<ImagePreviewDialog src={SOURCE} alt="Attached screenshot" />);
  const trigger = screen.getByRole("button", { name: "Open Attached screenshot" });
  await act(async () => fireEvent.click(trigger));
  const dialog = screen.getByRole("dialog", { name: "Image preview" });
  expect(
    within(dialog).getByRole("img", { name: "Full size Attached screenshot" }).getAttribute("src"),
  ).toBe(SOURCE);
  await act(async () => fireEvent.click(within(dialog).getByRole("button", { name: "Close" })));
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(screen.getByRole("button", { name: "Open Attached screenshot" })).toBe(trigger);
});

it("keeps noninteractive images outside the dialog interaction", () => {
  render(<ImagePreviewDialog src={SOURCE} alt="Static screenshot" interactive={false} />);
  const image = screen.getByRole("img", { name: "Static screenshot" });
  fireEvent.click(image);
  expect(image.getAttribute("src")).toBe(SOURCE);
  expect(image.getAttribute("draggable")).toBe("false");
  expect(screen.queryByRole("button")).toBeNull();
  expect(screen.queryByRole("dialog")).toBeNull();
});

// @covers AC-AGENTS-SHARED-IMAGE-INSPECTION-001.1, AC-AGENTS-SHARED-IMAGE-INSPECTION-001.3
it("switches inspection modes and resets on source replacement and reopening", async () => {
  const view = render(<ImagePreviewDialog src={SOURCE} alt="Screenshot" />);
  const trigger = screen.getByRole("button", { name: "Open Screenshot" });
  await act(async () => fireEvent.click(trigger));
  let dialog = screen.getByRole("dialog");
  const originalImage = within(dialog).getByRole("img");
  expect(within(dialog).getByRole("button", { name: "Fit" }).getAttribute(PRESSED)).toBe("true");
  await act(async () =>
    fireEvent.click(within(dialog).getByRole("button", { name: "Actual size" })),
  );
  expect(within(dialog).getByRole("button", { name: "Actual size" }).getAttribute(PRESSED)).toBe(
    "true",
  );
  expect(within(dialog).getByRole("img").getAttribute("src")).toBe(SOURCE);
  expect(within(dialog).getByRole("img")).toBe(originalImage);
  view.rerender(<ImagePreviewDialog src="blob:replacement" alt="Screenshot" />);
  dialog = screen.getByRole("dialog");
  expect(within(dialog).getByRole("img").getAttribute("src")).toBe("blob:replacement");
  expect(within(dialog).getByRole("button", { name: "Fit" }).getAttribute(PRESSED)).toBe("true");
  await act(async () =>
    fireEvent.click(within(dialog).getByRole("button", { name: "Actual size" })),
  );
  await act(async () => fireEvent.click(within(dialog).getByRole("button", { name: "Close" })));
  await act(async () => fireEvent.click(trigger));
  expect(
    within(screen.getByRole("dialog")).getByRole("button", { name: "Fit" }).getAttribute(PRESSED),
  ).toBe("true");
});

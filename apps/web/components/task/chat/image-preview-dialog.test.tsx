import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { ImagePreviewDialog } from "./image-preview-dialog";

const SOURCE = "data:image/png;base64,aW1hZ2U=";
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

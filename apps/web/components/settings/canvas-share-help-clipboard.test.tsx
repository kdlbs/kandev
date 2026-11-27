import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ExportReview } from "@/lib/api/domains/canvas-distribution-api";
import { CanvasShareHelp } from "./canvas-share-help";

const originalClipboard = Object.getOwnPropertyDescriptor(navigator, "clipboard");
const originalExecCommand = Object.getOwnPropertyDescriptor(document, "execCommand");
const copiedLabel = "Registry example copied";
const expectedExample = [
  "- id: my-canvas",
  "  repo: your-org/my-canvas",
  "  kind: canvas",
  "  previews:",
  "    - url: https://cdn.example.com/my-canvas-cover.webp",
  "      alt: Canvas overview",
].join("\n");

function setClipboard(writeText?: (text: string) => Promise<void>) {
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: writeText ? { writeText } : undefined,
  });
}

function setLegacyCopy(success: boolean) {
  const copies: string[] = [];
  const exec = vi.fn(() => {
    copies.push(document.querySelector("textarea")?.value ?? "");
    return success;
  });
  Object.defineProperty(document, "execCommand", { configurable: true, value: exec });
  return { exec, copies };
}

function openExample(review: ExportReview | null = null) {
  render(<CanvasShareHelp review={review} />);
  fireEvent.click(screen.getByText("How to share"));
  return screen.getByRole("button", { name: "Copy registry example" });
}

afterEach(() => {
  cleanup();
  if (originalClipboard) Object.defineProperty(navigator, "clipboard", originalClipboard);
  else Reflect.deleteProperty(navigator, "clipboard");
  if (originalExecCommand) Object.defineProperty(document, "execCommand", originalExecCommand);
  else Reflect.deleteProperty(document, "execCommand");
  vi.restoreAllMocks();
});

// @covers AC-CANVASES-MARKETPLACE-003.5
describe("Canvas registry example copy", () => {
  it("copies the displayed reviewed example through the modern API", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    setClipboard(writeText);
    const { exec } = setLegacyCopy(true);
    const review: ExportReview = {
      preparation_id: "prep",
      canvas_id: "canvas",
      workspace_id: "workspace",
      release_id: "release",
      metadata: { package_id: "reviewed-canvas" },
      sha256: "digest",
      files: [],
      bundle_bytes: 0,
      source_bytes: 0,
      expires_at: "",
      bundle_download: "",
      source_download: "",
    };
    fireEvent.click(openExample(review));
    const reviewedExample = expectedExample.replace("my-canvas\n", "reviewed-canvas\n");
    expect(document.querySelector("pre")?.textContent).toBe(reviewedExample);
    expect(writeText).toHaveBeenCalledWith(reviewedExample);
    await waitFor(() => expect(screen.getByRole("button", { name: copiedLabel })).toBeTruthy());
    expect(exec).not.toHaveBeenCalled();
  });

  it("recovers a rejected modern write using the legacy copy", async () => {
    setClipboard(vi.fn().mockRejectedValue(new DOMException("Denied", "NotAllowedError")));
    const { copies } = setLegacyCopy(true);
    fireEvent.click(openExample());
    await waitFor(() => expect(screen.getByRole("button", { name: copiedLabel })).toBeTruthy());
    expect(copies).toEqual([expectedExample]);
  });

  it("copies when the modern API is unavailable", async () => {
    setClipboard();
    const { copies } = setLegacyCopy(true);
    fireEvent.click(openExample());
    await waitFor(() => expect(screen.getByRole("button", { name: copiedLabel })).toBeTruthy());
    expect(copies).toEqual([expectedExample]);
  });

  it("clears earlier success when both copy paths fail", async () => {
    const writeText = vi
      .fn()
      .mockResolvedValueOnce(undefined)
      .mockRejectedValueOnce(new DOMException("Denied", "NotAllowedError"));
    setClipboard(writeText);
    const { exec } = setLegacyCopy(false);
    fireEvent.click(openExample());
    const copied = await screen.findByRole("button", { name: copiedLabel });
    fireEvent.click(copied);
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Copy registry example" })).toBeTruthy(),
    );
    expect(exec).toHaveBeenCalledWith("copy");
  });
});

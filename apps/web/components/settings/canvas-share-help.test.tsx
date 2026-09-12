import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

import { CanvasShareHelp } from "./canvas-share-help";

afterEach(() => cleanup());

describe("CanvasShareHelp", () => {
  it("shows a copyable canvas registry example", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
    render(<CanvasShareHelp review={null} />);

    expect(screen.getByText(/kind: canvas/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "canvases:copyRegistryExample" }));
    expect(writeText).toHaveBeenCalledWith(expect.stringContaining("previews:"));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "canvases:registryExampleCopied" })).toBeTruthy(),
    );
  });
});

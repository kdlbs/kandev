import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { KandevRendererProps } from "../types";

const parseRichOutput = vi.hoisted(() =>
  vi.fn(() => ({ version: 1 as const, title: "Stable result", blocks: [] })),
);

vi.mock("./parse", () => ({ parseRichOutput }));

import { RichOutputRenderer } from "./rich-output-renderer";

function RendererHarness(props: KandevRendererProps) {
  return RichOutputRenderer(props);
}

afterEach(() => {
  cleanup();
  parseRichOutput.mockClear();
});

describe("RichOutputRenderer", () => {
  it("reuses parsed output while persisted arguments and result stay unchanged", () => {
    const args = { version: 1, title: "Stable result", blocks: [] };
    const result = { version: 1 };
    const props: KandevRendererProps = { args, result, status: "complete" };
    const view = render(<RendererHarness {...props} />);

    view.rerender(<RendererHarness {...props} />);

    expect(parseRichOutput).toHaveBeenCalledOnce();
  });
});

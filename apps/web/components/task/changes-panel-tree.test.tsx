import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ComponentProps } from "react";
import { TreeDirRow } from "./changes-panel-tree";
import type { ChangesTreeNode } from "./changes-file-tree-model";

afterEach(cleanup);

type Props = ComponentProps<typeof TreeDirRow>;

function row(overrides: Partial<Props["row"]> = {}): Props["row"] {
  const node: ChangesTreeNode = { name: "src", path: "src", isDir: true, children: [] };
  return {
    node,
    chainRoot: node,
    displayName: "src",
    path: "src",
    depth: 2,
    isExpanded: true,
    isDir: true,
    ...overrides,
  };
}

describe("TreeDirRow", () => {
  it("exposes expansion state and toggles the requested directory", () => {
    const onToggle = vi.fn();
    render(<TreeDirRow row={row({ isExpanded: false })} baseIndentPx={12} onToggle={onToggle} />);

    const toggle = screen.getByRole("button", { name: "src" });
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    expect(toggle.style.paddingLeft).toBe("40px");
    fireEvent.click(toggle);
    expect(onToggle).toHaveBeenCalledOnce();
  });

  it("keeps a 44px touch target for directory expansion", () => {
    render(<TreeDirRow row={row()} baseIndentPx={0} onToggle={() => undefined} />);

    const toggle = screen.getByRole("button", { name: "src" });
    expect(toggle.className).toContain("[@media(pointer:coarse)]:min-h-11");
    expect(toggle.className).not.toContain("max-md:min-h-11");
  });
});

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { FileChangeStatus } from "@/lib/utils/file-change-status";
import { FileStatusIcon } from "./file-status-icon";

afterEach(cleanup);

describe("FileStatusIcon", () => {
  it.each<[FileChangeStatus, string, string]>([
    ["added", "Added", "tabler-icon-plus"],
    ["untracked", "Untracked", "tabler-icon-plus"],
    ["modified", "Modified", "tabler-icon-circle-filled"],
    ["deleted", "Deleted", "tabler-icon-minus"],
    ["renamed", "Moved", "tabler-icon-arrow-right"],
  ])("renders an accessible %s marker", (status, label, glyphClass) => {
    render(<FileStatusIcon status={status} />);

    const marker = screen.getByRole("img", { name: label });
    expect(marker.getAttribute("data-file-status")).toBe(status);
    expect(marker.getAttribute("title")).toBe(label);
    expect(marker.querySelector(`.${glyphClass}`)).not.toBeNull();
  });

  it("exposes previous-path context for a moved file", () => {
    render(<FileStatusIcon status="renamed" oldPath="src/old-name.ts" />);

    const marker = screen.getByRole("img", { name: "Moved from src/old-name.ts" });
    expect(marker.getAttribute("title")).toBe("Moved from src/old-name.ts");
  });

  it("merges caller classes into the fixed-size marker", () => {
    render(<FileStatusIcon status="modified" className="custom-marker" />);

    const marker = screen.getByRole("img", { name: "Modified" });
    expect(marker.className).toContain("shrink-0");
    expect(marker.className).toContain("custom-marker");
  });
});

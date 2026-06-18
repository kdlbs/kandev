import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { Message } from "@/lib/types/http";

// useOpenFileAtLine pulls in Monaco; stub it with a passthrough that forwards the
// (already selector-stripped) path so we can assert what gets opened.
vi.mock("@/hooks/use-file-editors", () => ({
  useOpenFileAtLine: (onOpenFile: ((path: string) => void) | undefined) => (path: string) =>
    onOpenFile?.(path),
}));

import { ToolReadMessage } from "./tool-read-message";

function readComment(filePath: string, offset?: number, limit?: number): Message {
  return {
    id: "m1",
    metadata: {
      status: "complete",
      normalized: { read_file: { file_path: filePath, offset, limit } },
    },
  } as unknown as Message;
}

describe("ToolReadMessage", () => {
  afterEach(cleanup);

  it("renders one openable link for a single-file read", () => {
    const openFile = vi.fn();
    render(
      <ToolReadMessage comment={readComment("apps/web/lib/utils.ts", 50)} onOpenFile={openFile} />,
    );

    const links = screen.getAllByRole("button");
    expect(links).toHaveLength(1);
    fireEvent.click(links[0]);
    expect(openFile).toHaveBeenCalledWith("apps/web/lib/utils.ts");
  });

  it("splits a comma-joined multi-file read into one link per file", () => {
    const openFile = vi.fn();
    render(
      <ToolReadMessage
        comment={readComment(
          "deployments/values.backupprod.yaml:1-80,deployments/values.au-backupprod.yaml:1-80",
        )}
        onOpenFile={openFile}
      />,
    );

    expect(screen.getByText("deployments/values.backupprod.yaml")).toBeTruthy();
    expect(screen.getByText("deployments/values.au-backupprod.yaml")).toBeTruthy();

    // Each link opens its bare path (no embedded line numbers).
    fireEvent.click(screen.getByText("deployments/values.au-backupprod.yaml"));
    expect(openFile).toHaveBeenCalledWith("deployments/values.au-backupprod.yaml");
    expect(openFile).not.toHaveBeenCalledWith("deployments/values.au-backupprod.yaml:1-80");
  });
});

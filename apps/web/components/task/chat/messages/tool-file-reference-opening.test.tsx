import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { consumePendingCursorPosition } from "@/hooks/file-editor-cursor";
import type { Message } from "@/lib/types/http";
import { ToolReadMessage } from "./tool-read-message";
import { ToolSearchMessage } from "./tool-search-message";

function readMessage(path: string, offset?: number): Message {
  return {
    id: "read",
    metadata: { status: "complete", normalized: { read_file: { file_path: path, offset } } },
  } as unknown as Message;
}

function searchMessage(files: string[]): Message {
  return {
    id: "search",
    metadata: {
      status: "complete",
      normalized: { code_search: { path: "/workspace/scope", output: { files } } },
    },
  } as unknown as Message;
}

afterEach(cleanup);

describe("tool file references", () => {
  // @covers AC-UI-TOOL-FILE-OPEN-001.1 AC-UI-TOOL-FILE-OPEN-001.2
  it.each(["Dockerfile", "Makefile", "README.md", ".env"])(
    "opens read target %s at its session-scoped line",
    (name) => {
      const open = vi.fn();
      const path = `/workspace/${name}`;
      render(
        <ToolReadMessage
          comment={readMessage(path, 35)}
          worktreePath="/workspace"
          sessionId="read-session"
          onOpenFile={open}
        />,
      );
      const link = screen.getByTitle(path);
      expect(link.tagName).toBe("BUTTON");
      fireEvent.click(link);
      expect(open).toHaveBeenCalledTimes(1);
      expect(open).toHaveBeenCalledWith(name);
      expect(consumePendingCursorPosition(name, undefined, "other-session")).toBeUndefined();
      expect(consumePendingCursorPosition(name, undefined, "read-session")).toEqual({
        line: 35,
        column: 1,
      });
    },
  );

  it.each(["LICENSE", "script", "app.tsx", ".config"])(
    "opens expanded search result %s with its original path",
    (name) => {
      const open = vi.fn();
      const path = `/workspace/${name}`;
      render(<ToolSearchMessage comment={searchMessage([path])} onOpenFile={open} />);
      fireEvent.click(screen.getByText("Found 1 file"));
      const link = screen.getByTitle(path);
      expect(link.tagName).toBe("BUTTON");
      fireEvent.click(link);
      expect(open).toHaveBeenCalledTimes(1);
      expect(open).toHaveBeenCalledWith(path);
      expect(screen.getByText("/workspace/scope").tagName).toBe("SPAN");
    },
  );

  // @covers AC-UI-TOOL-FILE-OPEN-001.3
  it.each([".", "/workspace/folder/", "   "])(
    "does not activate explicitly non-file search entry %j",
    (path) => {
      const open = vi.fn();
      const { container } = render(
        <ToolSearchMessage comment={searchMessage([path])} onOpenFile={open} />,
      );
      fireEvent.click(screen.getByText("Found 1 file"));
      const entry = Array.from(container.querySelectorAll("[title]")).find(
        (element) => element.getAttribute("title") === path,
      );
      expect(entry).toBeDefined();
      if (!entry) throw new Error("Missing search entry");
      expect(entry.tagName).toBe("SPAN");
      fireEvent.click(entry);
      expect(open).not.toHaveBeenCalled();
    },
  );

  it("keeps a read reference inactive without an opener", () => {
    render(<ToolReadMessage comment={readMessage("/workspace/Dockerfile", 35)} />);
    expect(screen.getByTitle("/workspace/Dockerfile").tagName).toBe("SPAN");
    expect(consumePendingCursorPosition("Dockerfile", undefined, "read-session")).toBeUndefined();
  });
});

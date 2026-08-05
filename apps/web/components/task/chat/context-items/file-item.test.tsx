import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { FileContextItem } from "@/lib/types/context";

const contextChipMock = vi.hoisted(() => vi.fn());
const lazyPreviewMock = vi.hoisted(() => vi.fn());

vi.mock("@tabler/icons-react", () => ({
  IconFile: () => <svg data-testid="file-icon" />,
  IconFolder: () => <svg data-testid="folder-icon" />,
}));

vi.mock("./context-chip", () => ({
  ContextChip: (props: Record<string, unknown>) => {
    contextChipMock(props);
    return (
      <div data-testid="context-chip">
        {props.leadingIcon as ReactNode}
        {props.preview as ReactNode}
      </div>
    );
  },
}));

vi.mock("./lazy-file-preview", () => ({
  LazyFilePreview: (props: Record<string, unknown>) => {
    lazyPreviewMock(props);
    return <div data-testid="lazy-file-preview" />;
  },
}));

import { FileItem } from "./file-item";

const FILE_PATH = "src/app.ts";

afterEach(() => {
  cleanup();
  contextChipMock.mockReset();
  lazyPreviewMock.mockReset();
});

function item(overrides: Record<string, unknown> = {}): FileContextItem {
  return {
    kind: "file",
    id: FILE_PATH,
    label: "app.ts",
    path: FILE_PATH,
    isDirectory: false,
    onOpen: vi.fn(),
    ...overrides,
  } as FileContextItem;
}

describe("FileItem", () => {
  it("keeps file previews and open behavior for file context", () => {
    const onOpen = vi.fn();
    const file = item({ onOpen });

    render(<FileItem item={file} sessionId="session-1" />);

    expect(lazyPreviewMock).toHaveBeenCalledWith({ path: FILE_PATH, sessionId: "session-1" });
    expect(contextChipMock).toHaveBeenCalledWith(
      expect.objectContaining({ kind: "file", leadingIcon: expect.anything() }),
    );
    expect(renderedIcon("file-icon")).toBe(true);
    const chipProps = contextChipMock.mock.calls[0]?.[0] as { onClick?: () => void };
    expect(chipProps.onClick).toEqual(expect.any(Function));
    chipProps.onClick?.();
    expect(onOpen).toHaveBeenCalledWith(FILE_PATH);
  });

  it("uses a folder icon and no preview or opener for directory context", () => {
    const directory = item({
      id: "src",
      label: "src",
      path: "src",
      isDirectory: true,
      onOpen: undefined,
    });

    render(<FileItem item={directory} sessionId="session-1" />);

    expect(lazyPreviewMock).not.toHaveBeenCalled();
    expect(contextChipMock).toHaveBeenCalledWith(
      expect.objectContaining({ kind: "file", onClick: undefined }),
    );
    expect(renderedIcon("folder-icon")).toBe(true);
    expect(renderedIcon("file-icon")).toBe(false);
  });
});

function renderedIcon(testId: string): boolean {
  return Boolean(document.querySelector(`[data-testid="${testId}"]`));
}

import type { DragEvent, KeyboardEvent } from "react";
import { describe, expect, it, vi } from "vitest";
import { droppedReferenceTokens, handleSuggestionKey } from "./passthrough-composer-state";

function applySelectedIndex(value: number | ((i: number) => number), current: number): number {
  return typeof value === "function" ? value(current) : value;
}

function dragEventWithData({
  text = "",
  uriList = "",
  files = [],
}: {
  text?: string;
  uriList?: string;
  files?: Array<{ name: string; webkitRelativePath?: string }>;
}): DragEvent {
  return {
    dataTransfer: {
      getData: (type: string) => {
        if (type === "text/plain") return text;
        if (type === "text/uri-list") return uriList;
        return "";
      },
      files,
    },
  } as unknown as DragEvent;
}

describe("droppedReferenceTokens", () => {
  it("splits plain text references across LF and CRLF lines", () => {
    const event = dragEventWithData({ text: "src/a.ts\r\n\nsrc/b.ts\n  src/c.ts  " });

    expect(droppedReferenceTokens(event)).toEqual(["src/a.ts", "src/b.ts", "src/c.ts"]);
  });

  it("uses URI list text when plain text is empty", () => {
    const event = dragEventWithData({ uriList: "file:///tmp/a.log\nfile:///tmp/b.log" });

    expect(droppedReferenceTokens(event)).toEqual(["file:///tmp/a.log", "file:///tmp/b.log"]);
  });

  it("falls back to dropped file paths or names", () => {
    const event = dragEventWithData({
      files: [
        { name: "ignored.txt", webkitRelativePath: "logs/ignored.txt" },
        { name: "fallback.txt" },
      ],
    });

    expect(droppedReferenceTokens(event)).toEqual(["logs/ignored.txt", "fallback.txt"]);
  });
});

describe("handleSuggestionKey", () => {
  it("short-circuits when suggestions are hidden", () => {
    const preventDefault = vi.fn();
    const insertSelection = vi.fn();

    const handled = handleSuggestionKey(
      { key: "Enter", preventDefault } as unknown as KeyboardEvent<HTMLTextAreaElement>,
      {
        showSuggestions: false,
        suggestionItems: ["/review"],
        selectedIndex: 0,
        setSelectedIndex: vi.fn(),
        insertSelection,
      },
    );

    expect(handled).toBe(false);
    expect(preventDefault).not.toHaveBeenCalled();
    expect(insertSelection).not.toHaveBeenCalled();
  });

  it("clamps arrow navigation at the list boundaries", () => {
    const preventDefault = vi.fn();
    const setSelectedIndex = vi.fn((value: number | ((i: number) => number)) =>
      applySelectedIndex(value, 1),
    );

    expect(
      handleSuggestionKey(
        { key: "ArrowDown", preventDefault } as unknown as KeyboardEvent<HTMLTextAreaElement>,
        {
          showSuggestions: true,
          suggestionItems: ["/review", "/resume"],
          selectedIndex: 1,
          setSelectedIndex,
          insertSelection: vi.fn(),
        },
      ),
    ).toBe(true);
    expect(setSelectedIndex.mock.results[0].value).toBe(1);

    setSelectedIndex.mockClear();
    expect(
      handleSuggestionKey(
        { key: "ArrowUp", preventDefault } as unknown as KeyboardEvent<HTMLTextAreaElement>,
        {
          showSuggestions: true,
          suggestionItems: ["/review", "/resume"],
          selectedIndex: 0,
          setSelectedIndex: vi.fn((value: number | ((i: number) => number)) =>
            applySelectedIndex(value, 0),
          ),
          insertSelection: vi.fn(),
        },
      ),
    ).toBe(true);
  });

  it("clamps selected suggestion when results shrink before Enter", () => {
    const preventDefault = vi.fn();
    const insertSelection = vi.fn();
    const handled = handleSuggestionKey(
      { key: "Enter", preventDefault } as unknown as KeyboardEvent<HTMLTextAreaElement>,
      {
        showSuggestions: true,
        suggestionItems: ["/review", "/resume"],
        selectedIndex: 4,
        setSelectedIndex: vi.fn(),
        insertSelection,
      },
    );

    expect(handled).toBe(true);
    expect(preventDefault).toHaveBeenCalled();
    expect(insertSelection).toHaveBeenCalledWith("/resume");
  });
});

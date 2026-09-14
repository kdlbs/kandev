import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  provider: "codemirror" as "codemirror" | "monaco",
  monacoImportCount: 0,
}));

vi.mock("@/hooks/use-editor-resolver", () => ({
  useEditorProvider: () => mocks.provider,
}));

vi.mock("@/components/editors/monaco/monaco-inline-diff", () => {
  mocks.monacoImportCount += 1;
  return {
    MonacoInlineDiff: () => <div data-testid="monaco-inline-diff" />,
  };
});

import { DiffViewBlock } from "./diff-view-block";

afterEach(() => {
  cleanup();
  mocks.provider = "codemirror";
  mocks.monacoImportCount = 0;
});

const DATA = {
  filePath: "src/example.ts",
  oldContent: "const before = true;",
  newContent: "const after = true;",
  diff: "",
  additions: 1,
  deletions: 1,
};

describe("DiffViewBlock editor loading", () => {
  it("does not load Monaco when another provider renders the diff", () => {
    render(<DiffViewBlock data={DATA} />);

    expect(mocks.monacoImportCount).toBe(0);
  });

  it("loads Monaco when it is the selected diff provider", async () => {
    mocks.provider = "monaco";
    render(<DiffViewBlock data={DATA} />);

    expect(await screen.findByTestId("monaco-inline-diff")).not.toBeNull();
    expect(mocks.monacoImportCount).toBe(1);
  });
});

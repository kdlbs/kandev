import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { editor, monacoModuleLoaded } = vi.hoisted(() => ({
  editor: { provider: "codemirror" },
  monacoModuleLoaded: vi.fn(),
}));

vi.mock("@/hooks/use-editor-resolver", () => ({
  useEditorProvider: () => editor.provider,
}));

vi.mock("@/components/editors/monaco/monaco-code-block", () => {
  monacoModuleLoaded();
  return { MonacoCodeBlock: () => <div data-testid="monaco-code-block" /> };
});

vi.mock("@/components/editors/codemirror/codemirror-code-block", () => ({
  CodeMirrorCodeBlock: () => <div data-testid="codemirror-code-block" />,
}));

vi.mock("@/components/editors/shiki/shiki-code-block", () => ({
  ShikiCodeBlock: () => <div data-testid="shiki-code-block" />,
}));

describe("CodeBlock editor loading", () => {
  beforeEach(() => {
    editor.provider = "codemirror";
    monacoModuleLoaded.mockClear();
    vi.resetModules();
  });

  it("keeps Monaco off the import graph until the configured provider needs it", async () => {
    const { CodeBlock } = await import("./code-block");
    render(<CodeBlock>const value = 1;</CodeBlock>);

    expect(screen.getByTestId("codemirror-code-block")).toBeTruthy();
    expect(monacoModuleLoaded).not.toHaveBeenCalled();
  });

  it("loads Monaco on demand", async () => {
    editor.provider = "monaco";
    const { CodeBlock } = await import("./code-block");
    render(<CodeBlock>const value = 1;</CodeBlock>);

    await waitFor(() => expect(screen.getByTestId("monaco-code-block")).toBeTruthy());
  });
});

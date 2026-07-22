import { beforeEach, describe, expect, it, vi } from "vitest";

const loaderState = vi.hoisted(() => ({
  attempts: 0,
  monaco: null as ReturnType<typeof createMonaco> | null,
}));

vi.mock("./monaco-loader", async () => {
  loaderState.attempts++;
  if (loaderState.attempts === 1) throw new Error("chunk unavailable");
  return { monaco: loaderState.monaco };
});

vi.mock("@/lib/lsp/lsp-client-manager", () => ({
  lspClientManager: { getFileOpener: vi.fn() },
}));

function createMonaco() {
  const defaults = {
    setCompilerOptions: vi.fn(),
    setDiagnosticsOptions: vi.fn(),
  };
  return {
    editor: {
      defineTheme: vi.fn(),
      registerEditorOpener: vi.fn(),
    },
    languages: {
      typescript: {
        typescriptDefaults: defaults,
        javascriptDefaults: defaults,
        JsxEmit: { ReactJSX: 4 },
        ScriptTarget: { ESNext: 99 },
        ModuleKind: { ESNext: 99 },
        ModuleResolutionKind: { NodeJs: 2 },
      },
    },
  };
}

describe("Monaco initialization", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.clearAllMocks();
    loaderState.attempts = 0;
    loaderState.monaco = null;
  });

  it("retries the shared loader after a transient import failure", async () => {
    const monaco = createMonaco();
    loaderState.monaco = monaco;
    const { waitForMonacoInstance } = await import("./monaco-init");

    await expect(waitForMonacoInstance()).rejects.toThrow();
    await expect(waitForMonacoInstance()).resolves.toBe(monaco);
    expect(loaderState.attempts).toBe(2);
  });
});

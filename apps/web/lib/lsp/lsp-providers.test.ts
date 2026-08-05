import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  getMonacoInstance: vi.fn(),
}));

vi.mock("@/components/editors/monaco/monaco-init", () => ({
  getMonacoInstance: mocks.getMonacoInstance,
}));

import { registerLspProviders } from "./lsp-providers";

type DefinitionProvider = {
  provideDefinition: (
    model: unknown,
    position: { lineNumber: number; column: number },
    token: { isCancellationRequested: boolean },
  ) => Promise<unknown>;
};

type CompletionProvider = {
  triggerCharacters?: string[];
  provideCompletionItems: (
    model: unknown,
    position: { lineNumber: number; column: number },
    context: { triggerKind: number; triggerCharacter?: string },
    token: { isCancellationRequested: boolean },
  ) => Promise<unknown>;
};

type SignatureHelpProvider = {
  signatureHelpTriggerCharacters?: string[];
  signatureHelpRetriggerCharacters?: string[];
};

type SemanticTokensProvider = {
  onDidChange: (listener: () => void) => { dispose: () => void };
  provideDocumentSemanticTokens: (
    model: unknown,
    lastResultId: string | null,
    token: { isCancellationRequested: boolean },
  ) => Promise<unknown>;
};

type MonacoLocation = {
  uri: { toString: () => string };
  range: {
    startLineNumber: number;
    startColumn: number;
    endLineNumber: number;
    endColumn: number;
  };
};

const SOURCE_URI = "file:///workspace/src/Source.kt";
const LOCATION_URI = "file:///workspace/src/LocationTarget.kt";
const LINK_URI = "file:///workspace/src/LinkTarget.kt";
const LOCATION_RANGE = {
  start: { line: 2, character: 3 },
  end: { line: 2, character: 9 },
};
const LINK_TARGET_RANGE = {
  start: { line: 4, character: 0 },
  end: { line: 8, character: 1 },
};
const LINK_SELECTION_RANGE = {
  start: { line: 6, character: 5 },
  end: { line: 6, character: 11 },
};
const MONACO_COMPLETION_ITEM_KIND = {
  Method: 0,
  Function: 1,
  Constructor: 2,
  Field: 3,
  Variable: 4,
  Class: 5,
  Struct: 6,
  Interface: 7,
  Module: 8,
  Property: 9,
  Event: 10,
  Operator: 11,
  Unit: 12,
  Value: 13,
  Constant: 14,
  Enum: 15,
  EnumMember: 16,
  Keyword: 17,
  Text: 18,
  Color: 19,
  File: 20,
  Reference: 21,
  Folder: 23,
  TypeParameter: 24,
  Snippet: 28,
} as const;

const rpc = { sendRequest: vi.fn() };
const ensureModelsExist = vi.fn();
const getModelUri = vi.fn((uri: string) => uri);
let definitionProvider: DefinitionProvider;
let completionProvider: CompletionProvider;
let semanticTokensProvider: SemanticTokensProvider;
let signatureHelpProvider: SignatureHelpProvider | undefined;
let registerSignatureHelpProvider: ReturnType<typeof vi.fn>;
const completionModel = {
  getWordUntilPosition: vi.fn(() => ({ word: "pri", startColumn: 4, endColumn: 7 })),
};

function disposable() {
  return { dispose: vi.fn() };
}

function monacoRange(range: typeof LOCATION_RANGE) {
  return {
    startLineNumber: range.start.line + 1,
    startColumn: range.start.character + 1,
    endLineNumber: range.end.line + 1,
    endColumn: range.end.character + 1,
  };
}

async function provideDefinition(result: unknown): Promise<MonacoLocation[] | null> {
  rpc.sendRequest.mockResolvedValue(result);
  return (await definitionProvider.provideDefinition(
    {},
    { lineNumber: 3, column: 7 },
    { isCancellationRequested: false },
  )) as MonacoLocation[] | null;
}

beforeEach(() => {
  vi.resetAllMocks();
  signatureHelpProvider = undefined;
  getModelUri.mockImplementation((uri: string) => uri);
  registerSignatureHelpProvider = vi.fn((_language: string, provider: SignatureHelpProvider) => {
    signatureHelpProvider = provider;
    return disposable();
  });
  const languages = {
    CompletionItemKind: MONACO_COMPLETION_ITEM_KIND,
    registerCompletionItemProvider: vi.fn((_language: string, provider: CompletionProvider) => {
      completionProvider = provider;
      return disposable();
    }),
    registerHoverProvider: vi.fn(() => disposable()),
    registerDefinitionProvider: vi.fn((_language: string, provider: DefinitionProvider) => {
      definitionProvider = provider;
      return disposable();
    }),
    registerReferenceProvider: vi.fn(() => disposable()),
    registerSignatureHelpProvider,
    registerDocumentSemanticTokensProvider: vi.fn(
      (_language: string, provider: SemanticTokensProvider) => {
        semanticTokensProvider = provider;
        return disposable();
      },
    ),
  };
  mocks.getMonacoInstance.mockReturnValue({
    languages,
    Uri: { parse: vi.fn((uri: string) => ({ toString: () => uri })) },
  });
  registerLspProviders({
    rpc,
    lspLanguage: "kotlin",
    serverCapabilities: null,
    semanticRefreshCallbacks: [],
    getDocumentUri: () => SOURCE_URI,
    getModelUri,
    ensureModelsExist,
  });
});

describe("LSP signature-help provider", () => {
  it("registers only when advertised and uses the server's trigger characters", () => {
    expect(registerSignatureHelpProvider).not.toHaveBeenCalled();

    registerLspProviders({
      rpc,
      lspLanguage: "kotlin",
      serverCapabilities: {
        signatureHelpProvider: {
          triggerCharacters: ["<", "("],
          retriggerCharacters: [",", ">"],
        },
      },
      semanticRefreshCallbacks: [],
      getDocumentUri: () => SOURCE_URI,
      getModelUri,
      ensureModelsExist,
    });

    expect(registerSignatureHelpProvider).toHaveBeenCalledOnce();
    expect(signatureHelpProvider).toMatchObject({
      signatureHelpTriggerCharacters: ["<", "("],
      signatureHelpRetriggerCharacters: [",", ">"],
    });
  });
});

describe("LSP completion provider", () => {
  it("registers only completion triggers advertised by the server", () => {
    expect(completionProvider.triggerCharacters).toBeUndefined();

    registerLspProviders({
      rpc,
      lspLanguage: "kotlin",
      serverCapabilities: {
        completionProvider: { triggerCharacters: [".", ":"] },
      },
      semanticRefreshCallbacks: [],
      getDocumentUri: () => SOURCE_URI,
      getModelUri,
      ensureModelsExist,
    });

    expect(completionProvider.triggerCharacters).toEqual([".", ":"]);
  });

  it("maps every standard LSP completion kind to Monaco's enum", async () => {
    const expectedKinds = [
      MONACO_COMPLETION_ITEM_KIND.Text,
      MONACO_COMPLETION_ITEM_KIND.Method,
      MONACO_COMPLETION_ITEM_KIND.Function,
      MONACO_COMPLETION_ITEM_KIND.Constructor,
      MONACO_COMPLETION_ITEM_KIND.Field,
      MONACO_COMPLETION_ITEM_KIND.Variable,
      MONACO_COMPLETION_ITEM_KIND.Class,
      MONACO_COMPLETION_ITEM_KIND.Interface,
      MONACO_COMPLETION_ITEM_KIND.Module,
      MONACO_COMPLETION_ITEM_KIND.Property,
      MONACO_COMPLETION_ITEM_KIND.Unit,
      MONACO_COMPLETION_ITEM_KIND.Value,
      MONACO_COMPLETION_ITEM_KIND.Enum,
      MONACO_COMPLETION_ITEM_KIND.Keyword,
      MONACO_COMPLETION_ITEM_KIND.Snippet,
      MONACO_COMPLETION_ITEM_KIND.Color,
      MONACO_COMPLETION_ITEM_KIND.File,
      MONACO_COMPLETION_ITEM_KIND.Reference,
      MONACO_COMPLETION_ITEM_KIND.Folder,
      MONACO_COMPLETION_ITEM_KIND.EnumMember,
      MONACO_COMPLETION_ITEM_KIND.Constant,
      MONACO_COMPLETION_ITEM_KIND.Struct,
      MONACO_COMPLETION_ITEM_KIND.Event,
      MONACO_COMPLETION_ITEM_KIND.Operator,
      MONACO_COMPLETION_ITEM_KIND.TypeParameter,
    ];
    rpc.sendRequest.mockResolvedValue(
      expectedKinds.map((_kind, index) => ({ label: `item-${index + 1}`, kind: index + 1 })),
    );

    const result = (await completionProvider.provideCompletionItems(
      completionModel,
      { lineNumber: 3, column: 7 },
      { triggerKind: 0 },
      { isCancellationRequested: false },
    )) as { suggestions: Array<{ kind: number }> };

    expect(result.suggestions.map((item) => item.kind)).toEqual(expectedKinds);
  });

  it("forwards trigger-character context using LSP enum values", async () => {
    rpc.sendRequest.mockResolvedValue([]);

    await completionProvider.provideCompletionItems(
      completionModel,
      { lineNumber: 3, column: 7 },
      { triggerKind: 1, triggerCharacter: "." },
      { isCancellationRequested: false },
    );

    expect(rpc.sendRequest).toHaveBeenCalledWith("textDocument/completion", {
      textDocument: { uri: SOURCE_URI },
      position: { line: 2, character: 6 },
      context: { triggerKind: 2, triggerCharacter: "." },
    });
  });

  it("forwards explicit invocations without a trigger character", async () => {
    rpc.sendRequest.mockResolvedValue([]);

    await completionProvider.provideCompletionItems(
      completionModel,
      { lineNumber: 3, column: 7 },
      { triggerKind: 0 },
      { isCancellationRequested: false },
    );

    expect(rpc.sendRequest).toHaveBeenCalledWith("textDocument/completion", {
      textDocument: { uri: SOURCE_URI },
      position: { line: 2, character: 6 },
      context: { triggerKind: 1 },
    });
  });

  it("maps incomplete-result retriggers to the LSP enum", async () => {
    rpc.sendRequest.mockResolvedValue([]);

    await completionProvider.provideCompletionItems(
      completionModel,
      { lineNumber: 3, column: 7 },
      { triggerKind: 2 },
      { isCancellationRequested: false },
    );

    expect(rpc.sendRequest).toHaveBeenCalledWith(
      "textDocument/completion",
      expect.objectContaining({ context: { triggerKind: 3 } }),
    );
  });
});

describe("LSP incomplete completion lists", () => {
  it("preserves the marker for follow-up requests", async () => {
    rpc.sendRequest.mockResolvedValue({
      isIncomplete: true,
      items: [{ label: "partialResult" }],
    });

    const result = await completionProvider.provideCompletionItems(
      completionModel,
      { lineNumber: 3, column: 7 },
      { triggerKind: 0 },
      { isCancellationRequested: false },
    );

    expect(result).toMatchObject({
      incomplete: true,
      suggestions: [{ label: "partialResult" }],
    });
  });
});

describe("LSP completion ranges", () => {
  it("uses the current Monaco word as the range when the server omits textEdit", async () => {
    rpc.sendRequest.mockResolvedValue([{ label: "println", insertText: "println" }]);

    const result = (await completionProvider.provideCompletionItems(
      completionModel,
      { lineNumber: 3, column: 7 },
      { triggerKind: 0 },
      { isCancellationRequested: false },
    )) as { suggestions: Array<{ range: unknown }> };

    expect(completionModel.getWordUntilPosition).toHaveBeenCalledWith({
      lineNumber: 3,
      column: 7,
    });
    expect(result.suggestions[0]?.range).toEqual({
      startLineNumber: 3,
      startColumn: 4,
      endLineNumber: 3,
      endColumn: 7,
    });
  });

  it("keeps the server textEdit range when one is provided", async () => {
    rpc.sendRequest.mockResolvedValue([
      {
        label: "println",
        textEdit: {
          range: {
            start: { line: 1, character: 2 },
            end: { line: 1, character: 5 },
          },
          newText: "println",
        },
      },
    ]);

    const result = (await completionProvider.provideCompletionItems(
      completionModel,
      { lineNumber: 3, column: 7 },
      { triggerKind: 0 },
      { isCancellationRequested: false },
    )) as { suggestions: Array<{ range: unknown }> };

    expect(result.suggestions[0]?.range).toEqual({
      startLineNumber: 2,
      startColumn: 3,
      endLineNumber: 2,
      endColumn: 6,
    });
  });

  it("maps an LSP insert/replace textEdit to Monaco's dual range", async () => {
    rpc.sendRequest.mockResolvedValue([
      {
        label: "println",
        textEdit: {
          insert: {
            start: { line: 1, character: 2 },
            end: { line: 1, character: 5 },
          },
          replace: {
            start: { line: 1, character: 2 },
            end: { line: 1, character: 9 },
          },
          newText: "println",
        },
      },
    ]);

    const result = (await completionProvider.provideCompletionItems(
      completionModel,
      { lineNumber: 3, column: 7 },
      { triggerKind: 0 },
      { isCancellationRequested: false },
    )) as { suggestions: Array<{ range: unknown }> };

    expect(result.suggestions[0]?.range).toEqual({
      insert: {
        startLineNumber: 2,
        startColumn: 3,
        endLineNumber: 2,
        endColumn: 6,
      },
      replace: {
        startLineNumber: 2,
        startColumn: 3,
        endLineNumber: 2,
        endColumn: 10,
      },
    });
  });
});

describe("LSP definition provider", () => {
  it("maps LocationLink arrays using their target selection ranges", async () => {
    const locations = await provideDefinition([
      {
        targetUri: LINK_URI,
        targetRange: LINK_TARGET_RANGE,
        targetSelectionRange: LINK_SELECTION_RANGE,
      },
    ]);

    expect(ensureModelsExist).toHaveBeenCalledWith([LINK_URI]);
    expect(locations?.map((location) => location.uri.toString())).toEqual([LINK_URI]);
    expect(locations?.[0]?.range).toEqual(monacoRange(LINK_SELECTION_RANGE));
  });

  it("maps a scalar LocationLink response", async () => {
    const locations = await provideDefinition({
      targetUri: LINK_URI,
      targetRange: LINK_TARGET_RANGE,
      targetSelectionRange: LINK_SELECTION_RANGE,
    });

    expect(ensureModelsExist).toHaveBeenCalledWith([LINK_URI]);
    expect(locations?.map((location) => location.uri.toString())).toEqual([LINK_URI]);
  });

  it("preserves mixed Location and LocationLink response order", async () => {
    const locations = await provideDefinition([
      { uri: LOCATION_URI, range: LOCATION_RANGE },
      {
        targetUri: LINK_URI,
        targetRange: LINK_TARGET_RANGE,
        targetSelectionRange: LINK_SELECTION_RANGE,
      },
    ]);

    expect(ensureModelsExist).toHaveBeenCalledWith([LOCATION_URI, LINK_URI]);
    expect(locations?.map((location) => location.uri.toString())).toEqual([LOCATION_URI, LINK_URI]);
    expect(locations?.map((location) => location.range)).toEqual([
      monacoRange(LOCATION_RANGE),
      monacoRange(LINK_SELECTION_RANGE),
    ]);
  });
});

describe("LSP semantic tokens provider", () => {
  it("returns an empty token payload without scheduling repeated refreshes", async () => {
    vi.useFakeTimers();
    try {
      registerLspProviders({
        rpc,
        lspLanguage: "kotlin",
        serverCapabilities: {
          semanticTokensProvider: {
            legend: { tokenTypes: [], tokenModifiers: [] },
            full: true,
          },
        },
        semanticRefreshCallbacks: [],
        getDocumentUri: () => SOURCE_URI,
        getModelUri,
        ensureModelsExist,
      });
      const onDidChange = vi.fn();
      semanticTokensProvider.onDidChange(onDidChange);
      rpc.sendRequest.mockResolvedValue({ data: [] });

      const result = await semanticTokensProvider.provideDocumentSemanticTokens({}, null, {
        isCancellationRequested: false,
      });

      expect(result).toEqual({ resultId: undefined, data: new Uint32Array() });
      await vi.advanceTimersByTimeAsync(5000);
      expect(onDidChange).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });
});

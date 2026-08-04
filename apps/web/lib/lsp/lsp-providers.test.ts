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
  provideCompletionItems: (
    model: unknown,
    position: { lineNumber: number; column: number },
    context: { triggerKind: number; triggerCharacter?: string },
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

const rpc = { sendRequest: vi.fn() };
const ensureModelsExist = vi.fn();
const getModelUri = vi.fn((uri: string) => uri);
let definitionProvider: DefinitionProvider;
let completionProvider: CompletionProvider;

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
  getModelUri.mockImplementation((uri: string) => uri);
  const languages = {
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
    registerSignatureHelpProvider: vi.fn(() => disposable()),
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

describe("LSP completion provider", () => {
  it("forwards trigger-character context using LSP enum values", async () => {
    rpc.sendRequest.mockResolvedValue([]);

    await completionProvider.provideCompletionItems(
      {},
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
      {},
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
      {},
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

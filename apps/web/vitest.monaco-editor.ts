// @ts-expect-error -- Monaco does not publish declarations for its internal URI module.
import { URI } from "monaco-editor/esm/vs/base/common/uri.js";

import type { Uri as MonacoUri } from "monaco-editor";

export const __KANDEV_VITEST_STUB__ = true;

type StubUri = { toString(): string };
type StubPosition = { lineNumber: number; column: number };
type StubRange = {
  startLineNumber: number;
  startColumn: number;
  endLineNumber: number;
  endColumn: number;
};
type StubModel = {
  uri: StubUri;
  setValue(value: string): void;
  getValue(): string;
  getLineContent(lineNumber: number): string;
  getWordUntilPosition(position: StubPosition): {
    startColumn: number;
    endColumn: number;
    word: string;
  };
  getValueInRange(range: StubRange): string;
  onDidChangeContent(listener: () => void): { dispose(): void };
  dispose(): void;
};

const noop = (..._args: unknown[]) => undefined;
const disposable = (..._args: unknown[]) => ({ dispose: noop });
const createLanguageDefaults = () => ({
  setCompilerOptions: noop,
  setDiagnosticsOptions: noop,
});

const models = new Map<string, StubModel>();
const modelMarkers = new Map<string, unknown[]>();
let nextModelId = 0;

function modelKey(uri: StubUri): string {
  return uri.toString();
}

function nextModelUri(): StubUri {
  nextModelId += 1;
  return URI.parse(`inmemory://vitest/model-${nextModelId}`) as StubUri;
}

function createModel(value = "", _language?: string, uri = nextModelUri()): StubModel {
  let currentValue = value;
  const listeners = new Set<() => void>();
  const key = modelKey(uri);
  const model: StubModel = {
    uri,
    setValue(nextValue) {
      currentValue = nextValue;
      for (const listener of listeners) listener();
    },
    getValue() {
      return currentValue;
    },
    getLineContent(lineNumber) {
      return currentValue.split(/\r?\n/)[lineNumber - 1] ?? "";
    },
    getWordUntilPosition({ lineNumber, column }) {
      const line = currentValue.split(/\r?\n/)[lineNumber - 1] ?? "";
      const beforeCursor = line.slice(0, column - 1);
      const match = beforeCursor.match(/[\w$]+$/);
      const word = match?.[0] ?? "";
      return { startColumn: column - word.length, endColumn: column, word };
    },
    getValueInRange(range) {
      const lines = currentValue
        .split(/\r?\n/)
        .slice(range.startLineNumber - 1, range.endLineNumber);
      if (lines.length === 0) return "";
      lines[0] = lines[0].slice(range.startColumn - 1);
      const lastLine = lines.length - 1;
      lines[lastLine] = lines[lastLine].slice(0, range.endColumn - 1);
      return lines.join("\n");
    },
    onDidChangeContent(listener) {
      listeners.add(listener);
      return { dispose: () => listeners.delete(listener) };
    },
    dispose() {
      models.delete(key);
      for (const markerKey of modelMarkers.keys()) {
        if (markerKey.startsWith(`${key}:`)) modelMarkers.delete(markerKey);
      }
    },
  };
  models.set(key, model);
  return model;
}

function getModel(uri: StubUri): StubModel | null {
  return models.get(modelKey(uri)) ?? null;
}

function setModelMarkers(model: StubModel, owner: string, markers: unknown[]): void {
  const key = `${modelKey(model.uri)}:${owner}`;
  if (markers.length === 0) {
    modelMarkers.delete(key);
    return;
  }
  modelMarkers.set(key, [...markers]);
}

function getModelMarkers(filter: { resource?: StubUri } = {}): unknown[] {
  const resourceKey = filter.resource ? `${modelKey(filter.resource)}:` : null;
  return [...modelMarkers.entries()]
    .filter(([key]) => resourceKey === null || key.startsWith(resourceKey))
    .flatMap(([, markers]) => markers);
}

const completionItemKind = {
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
};

export const Uri = URI as typeof MonacoUri;

export const editor = {
  defineTheme: noop,
  registerEditorOpener: disposable,
  createModel,
  getModels: () => [...models.values()],
  getModel,
  setModelMarkers,
  getModelMarkers,
};

export const languages = {
  CompletionItemKind: completionItemKind,
  registerCompletionItemProvider: disposable,
  registerDefinitionProvider: disposable,
  registerHoverProvider: disposable,
  registerReferenceProvider: disposable,
  registerSignatureHelpProvider: disposable,
  typescript: {
    javascriptDefaults: createLanguageDefaults(),
    typescriptDefaults: createLanguageDefaults(),
    JsxEmit: { ReactJSX: 4 },
    ModuleKind: { ESNext: 99 },
    ModuleResolutionKind: { NodeJs: 2 },
    ScriptTarget: { ESNext: 99 },
  },
};

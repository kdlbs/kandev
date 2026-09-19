import { describe, expect, it } from "vitest";
import * as monacoImport from "monaco-editor";

const monaco = monacoImport as typeof monacoImport & {
  __KANDEV_VITEST_STUB__?: boolean;
};
const typeScriptLanguages = monaco.languages.typescript as unknown as {
  javascriptDefaults?: unknown;
  typescriptDefaults?: unknown;
  JsxEmit?: { ReactJSX?: number };
  ScriptTarget?: { ESNext?: number };
  ModuleKind?: { ESNext?: number };
  ModuleResolutionKind?: { NodeJs?: number };
};
const monacoEditor = monaco.editor as unknown as Record<string, unknown>;
const monacoLanguages = monaco.languages as unknown as {
  CompletionItemKind?: { File?: number };
};

type StubModel = {
  uri: { toString(): string };
  setValue(value: string): void;
  getValue(): string;
  dispose(): void;
};
const modelEditor = monaco.editor as unknown as {
  createModel(value: string, language?: string, uri?: { toString(): string }): StubModel;
  getModel(uri: { toString(): string }): StubModel | null;
  setModelMarkers(model: StubModel, owner: string, markers: unknown[]): void;
  getModelMarkers(filter?: { resource?: { toString(): string } }): unknown[];
};

describe("Vitest Monaco boundary", () => {
  it("uses the unit-test stub with the required runtime contract", () => {
    expect({
      isStub: monaco.__KANDEV_VITEST_STUB__,
      uri: monaco.Uri.parse("file:///workspace/source.ts").toString(),
      hasTypeScriptDefaults: Boolean(typeScriptLanguages.typescriptDefaults),
      hasJavaScriptDefaults: Boolean(typeScriptLanguages.javascriptDefaults),
      hasIndependentLanguageDefaults:
        typeScriptLanguages.typescriptDefaults !== typeScriptLanguages.javascriptDefaults,
      completionItemKindFile: monacoLanguages.CompletionItemKind?.File,
      jsxEmitReactJSX: typeScriptLanguages.JsxEmit?.ReactJSX,
      scriptTargetESNext: typeScriptLanguages.ScriptTarget?.ESNext,
      moduleKindESNext: typeScriptLanguages.ModuleKind?.ESNext,
      moduleResolutionNodeJs: typeScriptLanguages.ModuleResolutionKind?.NodeJs,
      hasModelApi: ["createModel", "getModels", "getModel", "setModelMarkers"].every(
        (method) => typeof monacoEditor[method] === "function",
      ),
    }).toEqual({
      isStub: true,
      uri: "file:///workspace/source.ts",
      hasTypeScriptDefaults: true,
      hasJavaScriptDefaults: true,
      hasIndependentLanguageDefaults: true,
      completionItemKindFile: 20,
      jsxEmitReactJSX: 4,
      scriptTargetESNext: 99,
      moduleKindESNext: 99,
      moduleResolutionNodeJs: 2,
      hasModelApi: true,
    });
  });

  it("supports the model and marker APIs used by LSP consumers", () => {
    const uri = monaco.Uri.parse("file:///workspace/model.ts");
    const model = modelEditor.createModel("before", "typescript", uri);
    const marker = { message: "problem" };

    expect(modelEditor.getModel(uri)).toBe(model);
    model.setValue("after");
    modelEditor.setModelMarkers(model, "lsp:test", [marker]);
    expect({
      value: model.getValue(),
      markers: modelEditor.getModelMarkers({ resource: uri }),
    }).toEqual({
      value: "after",
      markers: [marker],
    });

    model.dispose();
    expect(modelEditor.getModel(uri)).toBeNull();
  });
});

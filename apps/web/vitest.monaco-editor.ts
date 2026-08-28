// @ts-expect-error -- Monaco does not publish declarations for its internal URI module.
import { URI } from "monaco-editor/esm/vs/base/common/uri.js";

import type { Uri as MonacoUri } from "monaco-editor";

export const __KANDEV_VITEST_STUB__ = true;

const noop = () => undefined;
const disposable = () => ({ dispose: noop });
const languageDefaults = {
  setCompilerOptions: noop,
  setDiagnosticsOptions: noop,
};

export const Uri = URI as typeof MonacoUri;

export const editor = {
  defineTheme: noop,
  registerEditorOpener: disposable,
};

export const languages = {
  registerCompletionItemProvider: disposable,
  registerDefinitionProvider: disposable,
  registerHoverProvider: disposable,
  registerReferenceProvider: disposable,
  registerSignatureHelpProvider: disposable,
  typescript: {
    javascriptDefaults: languageDefaults,
    typescriptDefaults: languageDefaults,
    JsxEmit: { ReactJSX: 4 },
    ModuleKind: { ESNext: 99 },
    ModuleResolutionKind: { NodeJs: 2 },
    ScriptTarget: { ESNext: 99 },
  },
};

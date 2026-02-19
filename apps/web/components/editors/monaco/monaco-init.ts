import type { Monaco } from "@monaco-editor/react";
import type { editor as monacoEditor, Uri, IRange, IPosition } from "monaco-editor";
import { KANDEV_MONACO_DARK, KANDEV_MONACO_LIGHT } from "@/lib/theme/editor-theme";

let initialized = false;
let monacoInstance: Monaco | null = null;

export function initMonacoThemes() {
  if (initialized || typeof window === "undefined") return;
  initialized = true;

  // Dynamic import ensures monaco-loader runs (which sets MonacoEnvironment +
  // loader.config) before any <Editor> component mounts.
  import("./monaco-loader").then(({ monaco }) => {
    monacoInstance = monaco;

    monaco.editor.defineTheme("kandev-dark", KANDEV_MONACO_DARK);
    monaco.editor.defineTheme("kandev-light", KANDEV_MONACO_LIGHT);

    // Disable Monaco's built-in TS/JS diagnostics — they can't resolve project-level
    // path aliases (@/, tsconfig paths). LSP diagnostics are applied separately via
    // setModelMarkers() and work independently of this setting.
    setMonacoDiagnostics(false);

    // Enable JSX so .tsx/.jsx files parse correctly
    const tsDefaults = monaco.languages.typescript.typescriptDefaults;
    const jsDefaults = monaco.languages.typescript.javascriptDefaults;
    tsDefaults.setCompilerOptions({
      jsx: monaco.languages.typescript.JsxEmit.ReactJSX,
      allowJs: true,
      allowNonTsExtensions: true,
      target: monaco.languages.typescript.ScriptTarget.ESNext,
      module: monaco.languages.typescript.ModuleKind.ESNext,
      moduleResolution: monaco.languages.typescript.ModuleResolutionKind.NodeJs,
    });
    jsDefaults.setCompilerOptions({
      jsx: monaco.languages.typescript.JsxEmit.ReactJSX,
      allowJs: true,
      allowNonTsExtensions: true,
      target: monaco.languages.typescript.ScriptTarget.ESNext,
      module: monaco.languages.typescript.ModuleKind.ESNext,
    });

    // Register editor opener to intercept Go-to-Definition navigation.
    // When the LSP returns a file:// URI, this opens the file in a dockview tab
    // instead of Monaco trying to navigate to a non-existent model.
    // Dynamic import to avoid circular dependency (lsp-client-manager → monaco-init).
    import("@/lib/lsp/lsp-client-manager").then(({ lspClientManager }) => {
      monaco.editor.registerEditorOpener({
        openCodeEditor(
          _source: monacoEditor.ICodeEditor,
          resource: Uri,
          selectionOrPosition?: IRange | IPosition,
        ) {
          const uri = resource.toString();
          const opener = lspClientManager.getFileOpener();
          if (opener && uri.startsWith("file:")) {
            // Extract line/column from the selection or position
            let line: number | undefined;
            let column: number | undefined;
            if (selectionOrPosition) {
              if ("lineNumber" in selectionOrPosition) {
                // IPosition
                line = selectionOrPosition.lineNumber;
                column = selectionOrPosition.column;
              } else if ("startLineNumber" in selectionOrPosition) {
                // IRange
                line = selectionOrPosition.startLineNumber;
                column = selectionOrPosition.startColumn;
              }
            }
            opener(uri, line, column);
            return true;
          }
          return false;
        },
      });
    });
  });
}

export function getMonacoInstance(): Monaco | null {
  return monacoInstance;
}

export function setMonacoDiagnostics(enabled: boolean) {
  if (!monacoInstance) return;
  const tsDefaults = monacoInstance.languages.typescript.typescriptDefaults;
  const jsDefaults = monacoInstance.languages.typescript.javascriptDefaults;
  const diagOptions = {
    noSemanticValidation: !enabled,
    noSuggestionDiagnostics: !enabled,
    noSyntaxValidation: !enabled,
  };
  tsDefaults.setDiagnosticsOptions(diagOptions);
  jsDefaults.setDiagnosticsOptions(diagOptions);
}

/**
 * Suppress or restore Monaco's built-in TS/JS language providers (hover,
 * completions, definitions, references, etc.). When LSP is active these
 * should be suppressed to avoid duplicate results.
 *
 * Uses the shared flag in builtin-providers.ts which is checked both at
 * registration time (to decide whether to wrap a provider) and at call time
 * (to decide whether wrapped providers return empty results).
 */
export function setMonacoBuiltinProviders(enabled: boolean) {
  import("./builtin-providers").then(({ setBuiltinTsSuppressed }) => {
    setBuiltinTsSuppressed(!enabled);
  });
}

/**
 * Monaco language provider registration for the LSP client.
 * Extracted from lsp-client-manager.ts to keep file size under limits.
 */

import type { editor as monacoEditor, IDisposable, languages } from 'monaco-editor';
import { getMonacoInstance } from '@/components/editors/monaco/monaco-init';

type MonacoModule = typeof import('monaco-editor');

// ---------------------------------------------------------------------------
// Types re-exported from the manager (kept minimal to avoid circular deps)
// ---------------------------------------------------------------------------

type LspRange = { start: { line: number; character: number }; end: { line: number; character: number } };

type JsonRpcConnection = {
  sendRequest(method: string, params: unknown): Promise<unknown>;
};

type GetDocumentUri = (model: monacoEditor.ITextModel) => string | null;
type EnsureModelsExist = (uris: string[], connectionKey: string) => void;

/** Shared context for all provider registration functions. */
type ProviderCtx = {
  monaco: MonacoModule;
  lang: string;
  rpc: JsonRpcConnection;
  getDocumentUri: GetDocumentUri;
  ensureModelsExist: EnsureModelsExist;
  connectionKey: string;
};

// ---------------------------------------------------------------------------
// LSP ↔ Monaco helpers (duplicated from manager to avoid circular import)
// ---------------------------------------------------------------------------

function toMonacoRange(r: LspRange) {
  return {
    startLineNumber: r.start.line + 1,
    startColumn: r.start.character + 1,
    endLineNumber: r.end.line + 1,
    endColumn: r.end.character + 1,
  };
}

function toLspPosition(lineNumber: number, column: number) {
  return { line: lineNumber - 1, character: column - 1 };
}

function toMonacoCompletionKind(lspKind: number | undefined): number {
  const map: Record<number, number> = {
    1: 14, 2: 1, 3: 0, 4: 8, 5: 4, 6: 5, 7: 7, 8: 7, 9: 8, 10: 9,
    11: 12, 12: 14, 13: 15, 14: 17, 15: 27, 16: 19, 17: 20, 18: 21,
    19: 23, 20: 16, 21: 14, 22: 6, 23: 24, 24: 25, 25: 26,
  };
  return map[lspKind ?? 1] ?? 14;
}

function extractDocumentation(doc: unknown): string | { value: string } | undefined {
  if (typeof doc === 'string') return doc;
  if (doc && typeof doc === 'object' && 'value' in doc) {
    return { value: (doc as { value: string }).value };
  }
  return undefined;
}

function extractHoverContents(contents: unknown): { value: string }[] {
  if (typeof contents === 'string') return [{ value: contents }];
  if (contents && typeof contents === 'object' && 'value' in contents) {
    return [{ value: (contents as { value: string }).value }];
  }
  if (Array.isArray(contents)) {
    return contents.map((item) => {
      if (typeof item === 'string') return { value: item };
      if (item && typeof item === 'object' && 'value' in item) return { value: (item as { value: string }).value };
      return { value: String(item) };
    });
  }
  return [];
}

// ---------------------------------------------------------------------------
// Completion item conversion
// ---------------------------------------------------------------------------

type LspCompletionItem = {
  label: string | { label: string; detail?: string; description?: string };
  kind?: number;
  detail?: string;
  documentation?: unknown;
  insertText?: string;
  insertTextFormat?: number;
  textEdit?: { range: LspRange; newText: string };
  additionalTextEdits?: Array<{ range: LspRange; newText: string }>;
  sortText?: string;
  filterText?: string;
};

function mapCompletionItem(item: LspCompletionItem): languages.CompletionItem {
  const label = typeof item.label === 'string' ? item.label : item.label.label;
  const insertText = item.textEdit?.newText ?? item.insertText ?? label;
  const isSnippet = item.insertTextFormat === 2;
  return {
    label,
    kind: toMonacoCompletionKind(item.kind),
    detail: item.detail,
    documentation: extractDocumentation(item.documentation),
    insertText,
    insertTextRules: isSnippet ? 4 /* InsertAsSnippet */ : undefined,
    range: item.textEdit?.range ? toMonacoRange(item.textEdit.range) : undefined,
    sortText: item.sortText,
    filterText: item.filterText,
    additionalTextEdits: item.additionalTextEdits?.map((e) => ({
      range: toMonacoRange(e.range),
      text: e.newText,
    })),
  } as languages.CompletionItem;
}

// ---------------------------------------------------------------------------
// Provider registration
// ---------------------------------------------------------------------------

function registerCompletionProvider(ctx: ProviderCtx): IDisposable {
  const { monaco, lang, rpc, getDocumentUri } = ctx;
  return monaco.languages.registerCompletionItemProvider(lang, {
    triggerCharacters: ['.', ':', '<', '"', "'", '/', '@', '#', ' '],
    provideCompletionItems: async (model, position, _context, token) => {
      const uri = getDocumentUri(model);
      if (!uri) return { suggestions: [] };
      try {
        const result = await rpc.sendRequest('textDocument/completion', {
          textDocument: { uri },
          position: toLspPosition(position.lineNumber, position.column),
        });
        if (token.isCancellationRequested) return { suggestions: [] };
        const items = Array.isArray(result) ? result : ((result as { items?: unknown[] })?.items ?? []);
        return { suggestions: (items as LspCompletionItem[]).map(mapCompletionItem) };
      } catch {
        return { suggestions: [] };
      }
    },
  });
}

function registerHoverProvider(ctx: ProviderCtx): IDisposable {
  const { monaco, lang, rpc, getDocumentUri } = ctx;
  return monaco.languages.registerHoverProvider(lang, {
    provideHover: async (model, position, token) => {
      const uri = getDocumentUri(model);
      if (!uri) return null;
      try {
        const result = await rpc.sendRequest('textDocument/hover', {
          textDocument: { uri },
          position: toLspPosition(position.lineNumber, position.column),
        }) as { contents: unknown; range?: LspRange } | null;
        if (token.isCancellationRequested || !result) return null;
        return {
          range: result.range ? toMonacoRange(result.range) : undefined,
          contents: extractHoverContents(result.contents),
        };
      } catch {
        return null;
      }
    },
  });
}

function registerDefinitionProvider(ctx: ProviderCtx): IDisposable {
  const { monaco, lang, rpc, getDocumentUri, ensureModelsExist, connectionKey } = ctx;
  return monaco.languages.registerDefinitionProvider(lang, {
    provideDefinition: async (model, position, token) => {
      const uri = getDocumentUri(model);
      if (!uri) return null;
      try {
        const result = await rpc.sendRequest('textDocument/definition', {
          textDocument: { uri },
          position: toLspPosition(position.lineNumber, position.column),
        });
        if (token.isCancellationRequested || !result) return null;
        const defs = Array.isArray(result) ? result : [result];
        ensureModelsExist(defs.map((d: { uri: string }) => d.uri), connectionKey);
        return defs.map((d: { uri: string; range: LspRange }) => ({
          uri: monaco.Uri.parse(d.uri),
          range: toMonacoRange(d.range),
        }));
      } catch {
        return null;
      }
    },
  });
}

function registerReferenceProvider(ctx: ProviderCtx): IDisposable {
  const { monaco, lang, rpc, getDocumentUri, ensureModelsExist, connectionKey } = ctx;
  return monaco.languages.registerReferenceProvider(lang, {
    provideReferences: async (model, position, context, token) => {
      const uri = getDocumentUri(model);
      if (!uri) return null;
      try {
        const result = await rpc.sendRequest('textDocument/references', {
          textDocument: { uri },
          position: toLspPosition(position.lineNumber, position.column),
          context: { includeDeclaration: context.includeDeclaration },
        });
        if (token.isCancellationRequested || !result) return null;
        const refs = Array.isArray(result) ? result : [];
        ensureModelsExist(refs.map((r: { uri: string }) => r.uri), connectionKey);
        return refs.map((r: { uri: string; range: LspRange }) => ({
          uri: monaco.Uri.parse(r.uri),
          range: toMonacoRange(r.range),
        }));
      } catch {
        return null;
      }
    },
  });
}

function registerSignatureHelpProvider(ctx: ProviderCtx): IDisposable {
  const { monaco, lang, rpc, getDocumentUri } = ctx;
  return monaco.languages.registerSignatureHelpProvider(lang, {
    signatureHelpTriggerCharacters: ['(', ','],
    provideSignatureHelp: async (model, position) => {
      const uri = getDocumentUri(model);
      if (!uri) return null;
      try {
        type SigResult = {
          signatures: Array<{
            label: string;
            documentation?: unknown;
            parameters?: Array<{ label: string | [number, number]; documentation?: unknown }>;
          }>;
          activeSignature?: number;
          activeParameter?: number;
        };
        const result = await rpc.sendRequest('textDocument/signatureHelp', {
          textDocument: { uri },
          position: toLspPosition(position.lineNumber, position.column),
        }) as SigResult | null;
        if (!result || !result.signatures?.length) return null;
        return {
          value: {
            signatures: result.signatures.map((sig) => ({
              label: sig.label,
              documentation: extractDocumentation(sig.documentation),
              parameters: (sig.parameters ?? []).map((p) => ({
                label: p.label,
                documentation: typeof p.documentation === 'string' ? p.documentation : undefined,
              })),
            })),
            activeSignature: result.activeSignature ?? 0,
            activeParameter: result.activeParameter ?? 0,
          },
          dispose: () => {},
        };
      } catch {
        return null;
      }
    },
  });
}

function registerSemanticTokensProvider(
  ctx: ProviderCtx,
  serverCapabilities: Record<string, unknown> | null,
  semanticRefreshCallbacks: (() => void)[],
): IDisposable[] {
  const { monaco, lang, rpc, getDocumentUri } = ctx;
  const semTokensCap = serverCapabilities?.semanticTokensProvider as
    { legend?: { tokenTypes: string[]; tokenModifiers: string[] }; full?: boolean | object } | undefined;
  if (!semTokensCap?.legend || !semTokensCap.full) return [];

  const legend = semTokensCap.legend;
  const disposables: IDisposable[] = [];

  const listeners = new Set<() => void>();
  const onDidChange = (listener: () => void) => {
    listeners.add(listener);
    return { dispose: () => listeners.delete(listener) };
  };
  semanticRefreshCallbacks.push(() => {
    for (const l of listeners) l();
  });

  const retryTimers = new Set<ReturnType<typeof setTimeout>>();

  disposables.push(
    monaco.languages.registerDocumentSemanticTokensProvider(lang, {
      onDidChange,
      getLegend() { return legend; },
      provideDocumentSemanticTokens: async (model, _lastResultId, token) => {
        const uri = getDocumentUri(model);
        if (!uri) return null;
        try {
          const result = await rpc.sendRequest('textDocument/semanticTokens/full', {
            textDocument: { uri },
          }) as { resultId?: string; data: number[] } | null;
          if (token.isCancellationRequested) return null;
          if (!result?.data?.length) {
            const timer = setTimeout(() => {
              retryTimers.delete(timer);
              for (const l of listeners) l();
            }, 5000);
            retryTimers.add(timer);
            return null;
          }
          return { resultId: result.resultId, data: new Uint32Array(result.data) };
        } catch {
          return null;
        }
      },
      releaseDocumentSemanticTokens() {},
    })
  );
  disposables.push({ dispose: () => { for (const t of retryTimers) clearTimeout(t); retryTimers.clear(); } });

  return disposables;
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

export interface RegisterLspProvidersOptions {
  rpc: JsonRpcConnection;
  lspLanguage: string;
  connectionKey: string;
  serverCapabilities: Record<string, unknown> | null;
  semanticRefreshCallbacks: (() => void)[];
  getDocumentUri: GetDocumentUri;
  ensureModelsExist: EnsureModelsExist;
}

export function registerLspProviders(opts: RegisterLspProvidersOptions): IDisposable[] {
  const monaco = getMonacoInstance();
  if (!monaco) return [];

  const monacoLanguages = getMonacoLanguagesForLsp(opts.lspLanguage);
  const disposables: IDisposable[] = [];

  for (const lang of monacoLanguages) {
    const ctx: ProviderCtx = {
      monaco, lang, rpc: opts.rpc,
      getDocumentUri: opts.getDocumentUri,
      ensureModelsExist: opts.ensureModelsExist,
      connectionKey: opts.connectionKey,
    };
    disposables.push(registerCompletionProvider(ctx));
    disposables.push(registerHoverProvider(ctx));
    disposables.push(registerDefinitionProvider(ctx));
    disposables.push(registerReferenceProvider(ctx));
    disposables.push(registerSignatureHelpProvider(ctx));
    disposables.push(...registerSemanticTokensProvider(ctx, opts.serverCapabilities, opts.semanticRefreshCallbacks));
  }

  return disposables;
}

// ---------------------------------------------------------------------------
// Language mapping
// ---------------------------------------------------------------------------

function getMonacoLanguagesForLsp(lspLanguage: string): string[] {
  switch (lspLanguage) {
    case 'typescript': return ['typescript', 'javascript', 'typescriptreact', 'javascriptreact'];
    case 'go': return ['go'];
    case 'rust': return ['rust'];
    case 'python': return ['python'];
    default: return [];
  }
}

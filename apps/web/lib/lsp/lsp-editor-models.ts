import type { editor as monacoEditor } from "monaco-editor";
import {
  canonicalFileUri,
  documentUriForModel,
  modelUriForDocument,
  fileUrisEqual,
  resolveFileUriInWorkspace,
} from "./file-uri";
import { toMonacoRange, toMonacoSeverity } from "./lsp-json-rpc";
import type { ManagedLspConnection, PublishDiagnosticsParams } from "./lsp-client-types";

export function connectionDocumentUri(
  model: monacoEditor.ITextModel,
  connection: ManagedLspConnection,
): string | null {
  const uri = documentUriForModel(model.uri.toString(), connection.sessionId);
  if (!uri || !connection.workspaceUri) return null;
  return resolveFileUriInWorkspace(uri, connection.workspaceUri, connection.repositorySubpaths)
    ? uri
    : null;
}

export function connectionModelUri(
  documentUri: string,
  connection: ManagedLspConnection,
  models: readonly monacoEditor.ITextModel[] = [],
): string | null {
  const uri = canonicalFileUri(documentUri);
  if (
    !uri ||
    !connection.workspaceUri ||
    !resolveFileUriInWorkspace(uri, connection.workspaceUri, connection.repositorySubpaths)
  ) {
    return null;
  }
  const existingModel = models.find((model) => connectionModelMatchesUri(model, uri, connection));
  if (existingModel) return existingModel.uri.toString();
  return modelUriForDocument(uri, connection.sessionId);
}

export function connectionModelMatchesUri(
  model: monacoEditor.ITextModel,
  documentUri: string,
  connection: ManagedLspConnection,
): boolean {
  const modelDocumentUri = documentUriForModel(model.uri.toString(), connection.sessionId);
  return modelDocumentUri !== null && fileUrisEqual(modelDocumentUri, documentUri);
}

export function diagnosticMarkers(params: PublishDiagnosticsParams): monacoEditor.IMarkerData[] {
  return params.diagnostics.map((diagnostic) => ({
    message: diagnostic.message,
    severity: toMonacoSeverity(diagnostic.severity),
    ...toMonacoRange(diagnostic.range),
    source: diagnostic.source,
    code: diagnosticCode(diagnostic.code),
  }));
}

function diagnosticCode(code: unknown): string | undefined {
  if (typeof code === "object" && code !== null) {
    return String((code as { value: unknown }).value);
  }
  return code === undefined ? undefined : String(code);
}

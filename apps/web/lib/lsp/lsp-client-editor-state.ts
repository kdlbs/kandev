import type { editor as monacoEditor } from "monaco-editor";
import { getMonacoInstance } from "@/components/editors/monaco/monaco-init";
import {
  canonicalFileUri,
  documentUriForModel,
  fileUrisEqual,
  modelUriForDocument,
  resolveFileUriInWorkspace,
} from "./file-uri";
import { connectionModelMatchesUri, diagnosticMarkers } from "./lsp-editor-models";
import type { ManagedLspConnection, PublishDiagnosticsParams } from "./lsp-client-types";

type IsCurrentConnection = (connection: ManagedLspConnection) => boolean;

/** Owns Monaco placeholder models and diagnostic markers for LSP connections. */
export class LspClientEditorState {
  private placeholderModelOwners = new Map<string, Set<string>>();

  constructor(private readonly isCurrentConnection: IsCurrentConnection) {}

  ensureModelsExist(uris: string[], connection: ManagedLspConnection): void {
    if (!this.isCurrentConnection(connection)) return;
    const monaco = getMonacoInstance();
    if (!monaco) return;

    for (const fileUri of uris) {
      const canonicalUri = canonicalFileUri(fileUri);
      if (!canonicalUri) continue;
      if (
        !connection.workspaceUri ||
        !resolveFileUriInWorkspace(
          canonicalUri,
          connection.workspaceUri,
          connection.repositorySubpaths,
        )
      ) {
        continue;
      }
      const modelUri = modelUriForDocument(canonicalUri, connection.sessionId);
      const parsed = monaco.Uri.parse(modelUri);
      const existingModel = monaco.editor
        .getModels()
        .find((model: monacoEditor.ITextModel) =>
          connectionModelMatchesUri(model, canonicalUri, connection),
        );

      if (existingModel) {
        const owners = this.placeholderModelOwners.get(modelUri);
        if (!owners || owners.has(connection.ownerId)) continue;
        owners.add(connection.ownerId);
        this.loadPlaceholderContent(canonicalUri, modelUri, existingModel, connection);
        continue;
      }

      const placeholderModel = monaco.editor.createModel("", undefined, parsed);
      this.placeholderModelOwners.set(modelUri, new Set([connection.ownerId]));
      this.loadPlaceholderContent(canonicalUri, modelUri, placeholderModel, connection);
    }
  }

  private loadPlaceholderContent(
    documentUri: string,
    modelUri: string,
    placeholderModel: monacoEditor.ITextModel,
    connection: ManagedLspConnection,
  ): void {
    if (!connection.workspaceUri) return;
    const location = resolveFileUriInWorkspace(
      documentUri,
      connection.workspaceUri,
      connection.repositorySubpaths,
    );
    if (!location) return;

    Promise.all([import("@/lib/ws/connection"), import("@/lib/ws/workspace-files")])
      .then(([{ getWebSocketClient }, { requestFileContent }]) => {
        if (!this.isPlaceholderOwner(modelUri, connection)) return;
        const client = getWebSocketClient();
        if (!client) return;
        return requestFileContent(client, connection.sessionId, location.path, location.repo);
      })
      .then((response) => {
        if (!response || !this.isPlaceholderOwner(modelUri, connection)) return;
        const monaco = getMonacoInstance();
        const currentModel = monaco?.editor.getModel(placeholderModel.uri);
        if (currentModel === placeholderModel) placeholderModel.setValue(response.content);
      })
      .catch(() => {
        // Best effort — placeholder stays empty.
      });
  }

  private isPlaceholderOwner(modelUri: string, connection: ManagedLspConnection): boolean {
    return (
      this.isCurrentConnection(connection) &&
      this.placeholderModelOwners.get(modelUri)?.has(connection.ownerId) === true
    );
  }

  disposePlaceholderModel(modelUri: string): void {
    if (!this.placeholderModelOwners.delete(modelUri)) return;
    const monaco = getMonacoInstance();
    if (!monaco) return;
    monaco.editor.getModel(monaco.Uri.parse(modelUri))?.dispose();
  }

  promoteDocumentModel(sessionId: string, documentUri: string, text: string): void {
    const canonicalUri = canonicalFileUri(documentUri);
    if (!canonicalUri) return;
    const monaco = getMonacoInstance();
    const realModelUri = modelUriForDocument(canonicalUri, sessionId);
    let promoted = false;
    for (const placeholderUri of this.placeholderModelOwners.keys()) {
      const placeholderDocumentUri = documentUriForModel(placeholderUri, sessionId);
      if (!placeholderDocumentUri || !fileUrisEqual(placeholderDocumentUri, canonicalUri)) continue;
      this.placeholderModelOwners.delete(placeholderUri);
      promoted = true;
      if (monaco && placeholderUri !== realModelUri) {
        monaco.editor.getModel(monaco.Uri.parse(placeholderUri))?.dispose();
      }
    }
    if (!promoted || !monaco) return;
    monaco.editor.getModel(monaco.Uri.parse(realModelUri))?.setValue(text);
  }

  handleDiagnostics(connection: ManagedLspConnection, params: PublishDiagnosticsParams): void {
    const uri = canonicalFileUri(params.uri);
    if (!uri) return;
    const canonicalParams = { ...params, uri };
    connection.diagnosticsByUri.set(uri, canonicalParams);
    const monaco = getMonacoInstance();
    if (!monaco) return;

    const targetModels = monaco.editor
      .getModels()
      .filter((model: monacoEditor.ITextModel) =>
        connectionModelMatchesUri(model, uri, connection),
      );
    for (const model of targetModels) {
      this.applyDiagnostics(connection.ownerId, model, canonicalParams);
    }
  }

  applyCachedDiagnostics(connection: ManagedLspConnection, model: monacoEditor.ITextModel): void {
    for (const params of connection.diagnosticsByUri.values()) {
      if (connectionModelMatchesUri(model, params.uri, connection)) {
        this.applyDiagnostics(connection.ownerId, model, params);
      }
    }
  }

  disposeConnection(ownerId: string): void {
    const monaco = getMonacoInstance();
    for (const [uri, owners] of this.placeholderModelOwners) {
      owners.delete(ownerId);
      if (owners.size > 0) continue;
      monaco?.editor.getModel(monaco.Uri.parse(uri))?.dispose();
      this.placeholderModelOwners.delete(uri);
    }

    if (!monaco) return;
    const markerOwner = this.markerOwner(ownerId);
    for (const model of monaco.editor.getModels()) {
      monaco.editor.setModelMarkers(model, markerOwner, []);
    }
  }

  private applyDiagnostics(
    ownerId: string,
    targetModel: monacoEditor.ITextModel,
    params: PublishDiagnosticsParams,
  ): void {
    const monaco = getMonacoInstance();
    if (!monaco) return;
    monaco.editor.setModelMarkers(
      targetModel,
      this.markerOwner(ownerId),
      diagnosticMarkers(params),
    );
  }

  private markerOwner(ownerId: string): string {
    return `lsp:${ownerId}`;
  }
}

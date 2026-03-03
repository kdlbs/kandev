"use client";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import {
  useEditorResolverStore,
  type EditorContext,
  type EditorProvider,
} from "@/lib/state/editor-resolver-store";

const PROVIDER_LABELS: Record<EditorProvider, string> = {
  monaco: "Monaco",
  codemirror: "CodeMirror",
  "pierre-diffs": "Pierre Diffs",
  shiki: "Shiki",
  tiptap: "Tiptap",
};

const CONFIGURABLE_CONTEXTS: { context: EditorContext; label: string }[] = [
  { context: "code-editor", label: "Code Editor" },
  { context: "diff-viewer", label: "Diff Viewer" },
  { context: "chat-code-block", label: "Chat Code Blocks" },
  { context: "chat-diff", label: "Chat Diff Blocks" },
];

export function EditorProviderSection() {
  const providers = useEditorResolverStore((s) => s.providers);
  const setProvider = useEditorResolverStore((s) => s.setProvider);
  const getValidProviders = useEditorResolverStore((s) => s.getValidProviders);

  return (
    <div className="space-y-4" data-testid="editor-provider-section">
      <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        Editor Providers
      </div>
      <div className="text-xs text-muted-foreground">
        Choose which rendering engine to use for each editor context. Changes apply immediately.
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        {CONFIGURABLE_CONTEXTS.map(({ context, label }) => (
          <div
            key={context}
            className="rounded-lg border border-border/60 bg-background px-4 py-3 space-y-1.5"
            data-testid={`editor-provider-card-${context}`}
          >
            <div className="text-sm font-medium text-foreground">{label}</div>
            <Select
              value={providers[context]}
              onValueChange={(value) => setProvider(context, value as EditorProvider)}
            >
              <SelectTrigger className="cursor-pointer" data-testid={`editor-provider-select-${context}`}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {getValidProviders(context).map((provider) => (
                  <SelectItem key={provider} value={provider} className="cursor-pointer">
                    {PROVIDER_LABELS[provider]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        ))}
      </div>
    </div>
  );
}

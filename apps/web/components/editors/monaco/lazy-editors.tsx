import { lazy, Suspense, type ComponentProps } from "react";
import { useTranslation } from "react-i18next";
import { waitForMonacoInstance } from "./monaco-init";
import type { MonacoCodeEditor } from "./monaco-code-editor";
import type { MonacoDiffViewer } from "./monaco-diff-viewer";

const CodeEditor = lazy(async () => {
  const [module] = await Promise.all([import("./monaco-code-editor"), waitForMonacoInstance()]);
  return { default: module.MonacoCodeEditor };
});

const DiffViewer = lazy(async () => {
  const [module] = await Promise.all([import("./monaco-diff-viewer"), waitForMonacoInstance()]);
  return { default: module.MonacoDiffViewer };
});

function EditorLoading({ diff = false }: { diff?: boolean }) {
  const { t } = useTranslation();
  return (
    <div
      role="status"
      className="flex h-full items-center justify-center text-muted-foreground text-sm"
    >
      {diff ? t("editors:loadingDiff") : t("editors:loadingEditor")}
    </div>
  );
}

export function LazyMonacoCodeEditor(props: ComponentProps<typeof MonacoCodeEditor>) {
  return (
    <Suspense fallback={<EditorLoading />}>
      <CodeEditor {...props} />
    </Suspense>
  );
}

export function LazyMonacoDiffViewer(props: ComponentProps<typeof MonacoDiffViewer>) {
  return (
    <Suspense fallback={<EditorLoading diff />}>
      <DiffViewer {...props} />
    </Suspense>
  );
}

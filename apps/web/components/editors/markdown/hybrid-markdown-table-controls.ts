import { useEffect, useState } from "react";
import type { EditorModel, TableAstNode } from "@vscode/markdown-editor";
import type { HybridMarkdownTableEdgeChromeProps } from "./hybrid-markdown-table-edge";
import type { MarkdownSourceRange } from "./hybrid-markdown-editor";

type ActiveTableControlsHost = Pick<HybridMarkdownTableEdgeChromeProps, "host" | "tableKey">;

const ACTIVE_TABLE_EDGE_WRAPPER_CLASS = "kandev-markdown-table-edge-wrapper";

type ActiveTableHostWithWrapper = ActiveTableControlsHost & {
  wrapper: HTMLElement;
};

function ensureActiveTableHost(
  root: HTMLDivElement,
  lifecycleRef: { current: { model: EditorModel } | null },
): ActiveTableHostWithWrapper | null {
  const activeTable = root.querySelector<HTMLElement>(".md-table.md-block-active");
  const wrapper = activeTable?.closest<HTMLElement>(".md-table-wrapper");
  if (!wrapper || !root.contains(wrapper)) return null;

  const existing = Array.from(wrapper.children).find(
    (child): child is HTMLDivElement =>
      child instanceof HTMLDivElement && child.dataset.kandevTableControls === "true",
  );
  const host = existing ?? document.createElement("div");
  if (!existing) {
    host.dataset.kandevTableControls = "true";
    host.className = "kandev-markdown-table-controls-host";
    wrapper.prepend(host);
  }

  const activeBlock = lifecycleRef.current?.model.activeBlock.get();
  const range =
    activeBlock?.kind === "table" && lifecycleRef.current
      ? findActiveTableRange(lifecycleRef.current.model, activeBlock as TableAstNode)
      : undefined;
  return { host, tableKey: range ? `table-${range.start}` : "active-table", wrapper };
}

export function useActiveTableControlsHost(
  rootRef: { current: HTMLDivElement | null },
  enabled: boolean,
  lifecycleRef: { current: { model: EditorModel } | null },
): ActiveTableControlsHost | null {
  const [hostState, setHostState] = useState<ActiveTableControlsHost | null>(null);

  useEffect(() => {
    const root = rootRef.current;
    if (!root || !enabled) {
      setHostState(null);
      return;
    }

    let currentHost: HTMLDivElement | null = null;
    let currentWrapper: HTMLElement | null = null;
    const reconcile = () => {
      const next = ensureActiveTableHost(root, lifecycleRef);
      if (!next) {
        currentHost?.remove();
        currentWrapper?.classList.remove(ACTIVE_TABLE_EDGE_WRAPPER_CLASS);
        currentHost = null;
        currentWrapper = null;
        setHostState(null);
        return;
      }

      if (currentWrapper && currentWrapper !== next.wrapper) {
        currentWrapper.classList.remove(ACTIVE_TABLE_EDGE_WRAPPER_CLASS);
      }
      if (currentHost && currentHost !== next.host) currentHost.remove();
      if (!next.wrapper.classList.contains(ACTIVE_TABLE_EDGE_WRAPPER_CLASS)) {
        next.wrapper.classList.add(ACTIVE_TABLE_EDGE_WRAPPER_CLASS);
      }
      currentHost = next.host;
      currentWrapper = next.wrapper;
      setHostState((current) =>
        current?.host === next.host && current.tableKey === next.tableKey
          ? current
          : { host: next.host, tableKey: next.tableKey },
      );
    };

    reconcile();
    const observer = new MutationObserver(reconcile);
    observer.observe(root, { attributes: true, childList: true, subtree: true });
    return () => {
      observer.disconnect();
      currentHost?.remove();
      currentWrapper?.classList.remove(ACTIVE_TABLE_EDGE_WRAPPER_CLASS);
    };
  }, [enabled, lifecycleRef, rootRef]);

  return hostState;
}

export function findActiveTableRange(
  model: EditorModel,
  activeTable: TableAstNode,
): MarkdownSourceRange | undefined {
  let start = 0;
  for (const node of model.document.get().content) {
    if (node === activeTable) {
      return { start, endExclusive: start + node.length };
    }
    start += node.length;
  }
  return undefined;
}

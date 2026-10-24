import { useCallback, useRef } from "react";
import type { DockviewApi } from "dockview-react";
import { fromDockviewApi, type LayoutNode, type LayoutState } from "@/lib/state/layout-manager";

function nodeGeometry(node: LayoutNode): unknown {
  return node.type === "leaf"
    ? [node.size, node.group.id]
    : [node.size, node.children.map(nodeGeometry)];
}

/** Tabs and titles do not change split geometry. Group identity/order and extents do. */
function geometry(layout: LayoutState): string {
  return JSON.stringify([
    layout.rootOrientation,
    layout.columns.map((column) => [
      column.width,
      column.tree ? nodeGeometry(column.tree) : column.groups.map((group) => group.id),
    ]),
  ]);
}

function retainNodeSizes(captured: LayoutNode, source: LayoutNode): LayoutNode {
  if (captured.type === "branch" && source.type === "branch") {
    return {
      ...captured,
      size: source.size,
      children: captured.children.map((child, index) =>
        retainNodeSizes(child, source.children[index]),
      ),
    };
  }
  return { ...captured, size: source.size };
}

function retainGeometry(captured: LayoutState, source: LayoutState): LayoutState {
  const hasSourceWidths = source.columns.some((column) => column.width !== undefined);
  return {
    ...captured,
    columns: captured.columns.map((column, index) => {
      const original = source.columns[index];
      return {
        ...column,
        ...(hasSourceWidths ? { width: original.width } : {}),
        minWidth: original.minWidth,
        maxWidth: original.maxWidth,
        ...(column.tree && original.tree
          ? { tree: retainNodeSizes(column.tree, original.tree) }
          : {}),
      };
    }),
  };
}

/** Source extents outlive preview caps and host resizes, but not explicit geometry edits. */
export function useEditorGeometry() {
  const sourceRef = useRef<LayoutState | null>(null);
  const previewRef = useRef("");
  const measurePreview = useCallback((api: DockviewApi) => {
    previewRef.current = geometry(fromDockviewApi(api));
  }, []);
  const seedGeometry = useCallback(
    (api: DockviewApi, source: LayoutState) => {
      sourceRef.current = source;
      measurePreview(api);
    },
    [measurePreview],
  );
  const captureGeometry = useCallback((captured: LayoutState): LayoutState => {
    const measured = geometry(captured);
    const result =
      sourceRef.current && measured === previewRef.current
        ? retainGeometry(captured, sourceRef.current)
        : captured;
    sourceRef.current = result;
    previewRef.current = measured;
    return result;
  }, []);
  return { seedGeometry, measurePreview, captureGeometry };
}

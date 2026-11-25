import { useEffect, useRef, type DependencyList } from "react";
import type { editor as monacoEditor } from "monaco-editor";
import { createRoot, type Root } from "react-dom/client";
import type { ReactNode } from "react";

import { observeCommentViewZone } from "./observe-comment-view-zone";

type ViewZoneEntry = { id: string; root: Root; stopObserving: () => void };

type AddZoneFn = (afterLine: number, heightPx: number, content: ReactNode) => void;

/**
 * Manages Monaco ViewZones that render React components inline between editor lines.
 * Extracted from the diff viewer's inline-comment pattern.
 *
 * @param editor   The Monaco editor instance (null while mounting)
 * @param deps     Dependency list — zones are rebuilt whenever these change
 * @param buildZones  Callback that receives an `addZone` helper to create zones
 */
export function useEditorViewZoneComments(
  editor: monacoEditor.ICodeEditor | null,
  deps: DependencyList,
  buildZones: (addZone: AddZoneFn) => void,
): void {
  const zonesRef = useRef<ViewZoneEntry[]>([]);

  useEffect(() => {
    if (!editor) return;

    // Tear down old zones
    const oldZones = zonesRef.current;
    if (oldZones.length > 0) {
      editor.changeViewZones((accessor) => {
        for (const z of oldZones) {
          z.stopObserving();
          accessor.removeZone(z.id);
        }
      });
      const staleRoots = oldZones.map((z) => z.root);
      queueMicrotask(() => staleRoots.forEach((r) => r.unmount()));
      zonesRef.current = [];
    }

    // Build new zones
    const newZones: ViewZoneEntry[] = [];

    const addZone: AddZoneFn = (afterLine, heightPx, content) => {
      const domNode = document.createElement("div");
      domNode.style.zIndex = "10";
      const zoneContent = document.createElement("div");
      domNode.append(zoneContent);
      const root = createRoot(zoneContent);
      root.render(content);
      let zoneId = "";
      const zone = { afterLineNumber: afterLine, heightInPx: heightPx, domNode };
      editor.changeViewZones((accessor) => {
        zoneId = accessor.addZone(zone);
      });
      newZones.push({
        id: zoneId,
        root,
        stopObserving: observeCommentViewZone(editor, zone, zoneId, zoneContent),
      });
    };

    buildZones(addZone);
    zonesRef.current = newZones;

    return () => {
      for (const zone of newZones) zone.stopObserving();
      try {
        editor.changeViewZones((accessor) => {
          for (const z of newZones) accessor.removeZone(z.id);
        });
      } catch {
        /* editor may be disposed */
      }
      const roots = newZones.map((z) => z.root);
      queueMicrotask(() => roots.forEach((r) => r.unmount()));
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editor, ...deps]);
}

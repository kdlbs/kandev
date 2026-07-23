import { Extension, isNodeEmpty, type Editor } from "@tiptap/core";
import type { Node as ProseMirrorNode } from "@tiptap/pm/model";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";

type DynamicPlaceholderStorage = {
  dynamicPlaceholder: { text: string };
};

function placeholderStorage(editor: Editor): DynamicPlaceholderStorage {
  return editor.storage as unknown as DynamicPlaceholderStorage;
}

/** Placeholder extension backed by live editor storage, avoiding stale options snapshots. */
export const DynamicPlaceholder = Extension.create({
  name: "dynamicPlaceholder",

  addStorage() {
    return { text: "" };
  },

  addProseMirrorPlugins() {
    const editor = this.editor;
    return [
      new Plugin({
        key: new PluginKey("dynamicPlaceholder"),
        props: {
          decorations: ({ doc, selection }) => {
            if (!editor.isEditable && !editor.isEmpty) return null;
            const { anchor } = selection;
            const decorations: InstanceType<typeof Decoration>[] = [];
            const isEmptyDoc = editor.isEmpty;
            doc.descendants((node: ProseMirrorNode, pos: number) => {
              const hasAnchor = anchor >= pos && anchor <= pos + node.nodeSize;
              const isEmpty = !node.isLeaf && isNodeEmpty(node);
              if (hasAnchor && isEmpty) {
                const classes = ["is-empty"];
                if (isEmptyDoc) classes.push("is-editor-empty");
                decorations.push(
                  Decoration.node(pos, pos + node.nodeSize, {
                    class: classes.join(" "),
                    "data-placeholder": placeholderStorage(editor).dynamicPlaceholder.text,
                  }),
                );
              }
              return false;
            });
            return DecorationSet.create(doc, decorations);
          },
        },
      }),
    ];
  },
});

export function updateDynamicPlaceholder(editor: Editor | null, text: string) {
  if (!editor) return;
  placeholderStorage(editor).dynamicPlaceholder.text = text;
  editor.view.dispatch(editor.state.tr);
}

/**
 * TipTap node view extension for rendering mermaid diagrams.
 * Ported from the Milkdown mermaidCodeBlockView in markdown-editor.tsx.
 */

import { Node, mergeAttributes } from "@tiptap/core";
import { ReactNodeViewRenderer, NodeViewWrapper } from "@tiptap/react";
import { createElement, useEffect, useRef, useState } from "react";
import mermaid from "mermaid";

let mermaidInitialized = false;
let mermaidIdCounter = 0;

function initMermaid(theme: "dark" | "light" = "dark") {
  mermaid.initialize({
    startOnLoad: false,
    theme: theme === "dark" ? "dark" : "default",
    securityLevel: "loose",
  });
  mermaidInitialized = true;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function MermaidNodeView({ node }: { node: any }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [error, setError] = useState<string | null>(null);
  const code = node.textContent;

  useEffect(() => {
    if (!mermaidInitialized) initMermaid();
    if (!containerRef.current || !code.trim()) return;

    const id = `mermaid-${++mermaidIdCounter}`;
    mermaid
      .render(id, code)
      .then(({ svg }) => {
        if (containerRef.current) {
          containerRef.current.innerHTML = svg;
          setError(null);
        }
      })
      .catch((err: Error) => {
        setError(err.message);
      });
  }, [code]);

  return createElement(
    NodeViewWrapper,
    { className: "mermaid-container" },
    error
      ? createElement("pre", { className: "mermaid-error" }, `Error rendering diagram: ${error}`)
      : createElement("div", { ref: containerRef }),
  );
}

/**
 * Custom CodeBlock extension that renders mermaid blocks as diagrams.
 * For non-mermaid blocks, uses default rendering.
 */
export const MermaidCodeBlock = Node.create({
  name: "mermaidCodeBlock",
  group: "block",
  content: "text*",
  marks: "",
  code: true,
  defining: true,

  addAttributes() {
    return {
      language: {
        default: null,
        parseHTML: (element) => element.getAttribute("data-language"),
        renderHTML: (attributes) => {
          if (!attributes.language) return {};
          return { "data-language": attributes.language };
        },
      },
    };
  },

  parseHTML() {
    return [
      {
        tag: "pre",
        preserveWhitespace: "full",
        getAttrs: (node) => {
          const el = node as HTMLElement;
          const code = el.querySelector("code");
          const lang = code?.className?.match(/language-(\w+)/)?.[1];
          if (lang === "mermaid") return { language: "mermaid" };
          return false; // Let default code block handle non-mermaid
        },
      },
    ];
  },

  renderHTML({ HTMLAttributes }) {
    return ["pre", mergeAttributes(HTMLAttributes), ["code", {}, 0]];
  },

  addNodeView() {
    return ReactNodeViewRenderer(MermaidNodeView);
  },
});

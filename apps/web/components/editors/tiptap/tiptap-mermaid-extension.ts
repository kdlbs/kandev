/**
 * Extends CodeBlockLowlight with mermaid diagram rendering.
 * When a code block has language="mermaid", it renders as an SVG diagram
 * instead of syntax-highlighted code. Non-mermaid blocks render normally.
 */

import type { Node as PmNode } from "@tiptap/pm/model";
import type { NodeView } from "@tiptap/pm/view";
import CodeBlockLowlight from "@tiptap/extension-code-block-lowlight";
import mermaid from "mermaid";
import {
  DEFAULT_SCALE,
  SCALE_STEP,
  MIN_SCALE,
  MAX_SCALE,
  getSvgDimensions,
} from "@/components/shared/mermaid-utils";

/**
 * Known mermaid diagram type keywords. If a code block's first line starts
 * with one of these, it is treated as a mermaid diagram even without an
 * explicit ```mermaid language tag.
 */
const MERMAID_KEYWORDS = [
  "flowchart",
  "graph",
  "sequenceDiagram",
  "classDiagram",
  "stateDiagram",
  "erDiagram",
  "gantt",
  "pie",
  "gitgraph",
  "journey",
  "mindmap",
  "timeline",
  "quadrantChart",
  "xychart-beta",
  "block-beta",
  "sankey-beta",
  "packet-beta",
  "architecture-beta",
  "kanban",
  "requirement",
  "C4Context",
  "C4Container",
  "C4Component",
  "C4Dynamic",
  "C4Deployment",
  "zenuml",
];

const MERMAID_RE = new RegExp(`^\\s*(?:${MERMAID_KEYWORDS.join("|")})(?:\\s|$)`, "m");

/** Detect mermaid content by language tag OR by first-line keyword. */
export function isMermaidContent(language: string | null | undefined, text: string): boolean {
  if (language === "mermaid") return true;
  return MERMAID_RE.test(text);
}

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

/** SVG icon markup for the code toggle button (Tabler IconCode). */
const CODE_ICON_SVG = `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>`;

/** Mermaid keywords regex for syntax highlighting. */
const HIGHLIGHT_KEYWORDS = new RegExp(
  `\\b(${MERMAID_KEYWORDS.join("|")}|subgraph|end|participant|actor|loop|alt|else|opt|par|and|critical|break|rect|note|over|of|left|right|TD|TB|BT|RL|LR)\\b`,
  "g",
);
const HIGHLIGHT_ARROW =
  /(-->|==>|-.->|---->|===|---|==>|~~>|\|>|<\||\.\.\.|--[>x)]-?|--\|[^|]*\|)/g;
const HIGHLIGHT_LABEL =
  /(\[.*?\]|\(.*?\)|\{.*?\}|\[\[.*?\]\]|\(\(.*?\)\)|\[\/.*?\/\]|\[\\.*?\\\])/g;

/** Apply basic syntax highlighting to mermaid source code. */
function highlightMermaid(code: string): string {
  const escaped = code.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

  return escaped
    .replace(HIGHLIGHT_LABEL, '<span class="mm-label">$1</span>')
    .replace(HIGHLIGHT_ARROW, '<span class="mm-arrow">$1</span>')
    .replace(HIGHLIGHT_KEYWORDS, '<span class="mm-keyword">$1</span>');
}

/**
 * ProseMirror NodeView that renders mermaid diagrams with zoom controls
 * and a toggle to show the raw mermaid source code.
 */
class MermaidNodeView implements NodeView {
  dom: HTMLElement;
  private diagramView: HTMLElement;
  private codeView: HTMLElement;
  private svgContainer: HTMLElement;
  private sizeWrapper: HTMLElement;
  private toolbar: HTMLElement;
  private zoomLabel: HTMLButtonElement;
  private zoomOutBtn: HTMLButtonElement;
  private zoomInBtn: HTMLButtonElement;
  private separator: HTMLElement;
  private scale = DEFAULT_SCALE;
  private node: PmNode;
  private showingCode = false;

  constructor(node: PmNode) {
    this.node = node;

    // Root container
    this.dom = document.createElement("div");
    this.dom.className = "mermaid-container group";

    // --- Diagram view ---
    this.diagramView = document.createElement("div");
    this.diagramView.style.display = "inline-block";
    this.diagramView.style.position = "relative";

    this.svgContainer = document.createElement("div");
    this.svgContainer.style.transformOrigin = "top left";
    this.svgContainer.style.transform = `scale(${this.scale})`;

    this.sizeWrapper = document.createElement("div");
    this.sizeWrapper.style.overflow = "hidden";
    this.sizeWrapper.appendChild(this.svgContainer);

    const scrollWrapper = document.createElement("div");
    scrollWrapper.style.overflowX = "auto";
    scrollWrapper.style.padding = "0.5rem";
    scrollWrapper.appendChild(this.sizeWrapper);
    this.diagramView.appendChild(scrollWrapper);

    // --- Code view (hidden by default) ---
    this.codeView = document.createElement("pre");
    this.codeView.className = "mermaid-code-view";
    this.codeView.style.display = "none";
    const codeEl = document.createElement("code");
    codeEl.innerHTML = highlightMermaid(node.textContent);
    this.codeView.appendChild(codeEl);

    // --- Floating toolbar ---
    this.toolbar = document.createElement("div");
    this.toolbar.className = "mermaid-toolbar";
    // Prevent mousedown from stealing focus from the ProseMirror editor,
    // which would cause the document to scroll to the cursor/selection.
    this.toolbar.addEventListener("mousedown", (e) => e.preventDefault());

    this.zoomOutBtn = this.createBtn("−", () => this.setScale(this.scale - SCALE_STEP));
    this.zoomOutBtn.className = "mermaid-zoom-btn";

    this.zoomLabel = document.createElement("button");
    this.zoomLabel.type = "button";
    this.zoomLabel.className = "mermaid-zoom-label";
    this.zoomLabel.addEventListener("click", () => this.setScale(DEFAULT_SCALE));
    this.updateZoomLabel();

    this.zoomInBtn = this.createBtn("+", () => this.setScale(this.scale + SCALE_STEP));
    this.zoomInBtn.className = "mermaid-zoom-btn";

    this.separator = document.createElement("div");
    this.separator.className = "mermaid-toolbar-sep";

    const codeToggle = document.createElement("button");
    codeToggle.type = "button";
    codeToggle.className = "mermaid-zoom-btn";
    codeToggle.innerHTML = CODE_ICON_SVG;
    codeToggle.title = "Toggle code";
    codeToggle.addEventListener("click", () => this.toggleCode());

    this.toolbar.append(
      this.zoomOutBtn,
      this.zoomLabel,
      this.zoomInBtn,
      this.separator,
      codeToggle,
    );

    this.dom.append(this.diagramView, this.codeView, this.toolbar);
    this.renderDiagram(node.textContent);
  }

  private createBtn(text: string, onClick: () => void): HTMLButtonElement {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.textContent = text;
    btn.addEventListener("click", onClick);
    return btn;
  }

  private toggleCode() {
    this.showingCode = !this.showingCode;
    if (this.showingCode) {
      this.diagramView.style.display = "none";
      this.codeView.style.display = "";
      this.zoomOutBtn.style.display = "none";
      this.zoomLabel.style.display = "none";
      this.zoomInBtn.style.display = "none";
      this.separator.style.display = "none";
    } else {
      this.diagramView.style.display = "inline-block";
      this.codeView.style.display = "none";
      this.zoomOutBtn.style.display = "";
      this.zoomLabel.style.display = "";
      this.zoomInBtn.style.display = "";
      this.separator.style.display = "";
    }
  }

  private setScale(value: number) {
    this.scale = Math.max(MIN_SCALE, Math.min(MAX_SCALE, value));
    this.svgContainer.style.transform = `scale(${this.scale})`;
    this.updateSizeWrapper();
    this.updateZoomLabel();
  }

  private updateSizeWrapper() {
    const dims = getSvgDimensions(this.svgContainer);
    if (dims) {
      this.svgContainer.style.width = `${dims.w}px`;
      this.svgContainer.style.height = `${dims.h}px`;
      this.sizeWrapper.style.width = `${dims.w * this.scale}px`;
      this.sizeWrapper.style.height = `${dims.h * this.scale}px`;
    }
  }

  private updateZoomLabel() {
    this.zoomLabel.textContent = `${Math.round(this.scale * 100)}%`;
  }

  private renderDiagram(code: string) {
    if (!mermaidInitialized) initMermaid();
    if (!code.trim()) return;
    const id = `mermaid-${++mermaidIdCounter}`;
    mermaid
      .render(id, code)
      .then(({ svg }) => {
        this.svgContainer.innerHTML = svg;
        this.updateSizeWrapper();
      })
      .catch((err: Error) => {
        this.svgContainer.innerHTML = "";
        const pre = document.createElement("pre");
        pre.className = "mermaid-error";
        pre.textContent = `Error rendering diagram: ${err.message}`;
        this.svgContainer.appendChild(pre);
      });
  }

  update(node: PmNode): boolean {
    if (node.type.name !== "codeBlock") return false;
    if (!isMermaidContent(node.attrs.language, node.textContent)) return false;
    if (node.textContent !== this.node.textContent) {
      this.node = node;
      this.renderDiagram(node.textContent);
      const codeEl = this.codeView.querySelector("code");
      if (codeEl) codeEl.innerHTML = highlightMermaid(node.textContent);
    }
    return true;
  }

  ignoreMutation() {
    return true;
  }

  stopEvent() {
    return true;
  }
}

/**
 * Default code block NodeView that preserves normal ProseMirror rendering.
 * Provides `contentDOM` so ProseMirror manages the text content + lowlight
 * decorations still apply.
 */
class DefaultCodeBlockView implements NodeView {
  dom: HTMLElement;
  contentDOM: HTMLElement;

  constructor(node: PmNode) {
    this.dom = document.createElement("pre");
    this.dom.setAttribute("spellcheck", "false");
    this.contentDOM = document.createElement("code");
    if (node.attrs.language) {
      this.contentDOM.className = `language-${node.attrs.language}`;
    }
    this.dom.appendChild(this.contentDOM);
  }

  update(node: PmNode): boolean {
    if (node.type.name !== "codeBlock") return false;
    this.contentDOM.className = node.attrs.language ? `language-${node.attrs.language}` : "";
    return true;
  }
}

/**
 * Creates CodeBlockLowlight extended with mermaid rendering.
 * Pass the lowlight instance as you normally would to CodeBlockLowlight.
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function createCodeBlockWithMermaid(lowlight: any) {
  return CodeBlockLowlight.configure({ lowlight }).extend({
    addNodeView() {
      return ({ node }: { node: PmNode }) => {
        if (isMermaidContent(node.attrs.language, node.textContent)) {
          return new MermaidNodeView(node);
        }
        return new DefaultCodeBlockView(node);
      };
    },
  });
}

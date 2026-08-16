"use client";

import { NodeViewContent, NodeViewWrapper, type NodeViewProps } from "@tiptap/react";
import { useTranslation } from "react-i18next";

// `value` is the highlight.js language id and the language names are proper
// nouns, so both stay untranslated. Only the "no language" option is copy, and
// it travels as a catalog key because this table is built at module load.
// i18n-exempt: programming language names are not translated.
const CODE_LANGUAGES: Array<{ value: string; label?: string; labelKey?: string }> = [
  { value: "", labelKey: "task:codeLanguagePlain" },
  { value: "javascript", label: "JavaScript" },
  { value: "typescript", label: "TypeScript" },
  { value: "python", label: "Python" },
  { value: "go", label: "Go" },
  { value: "rust", label: "Rust" },
  { value: "java", label: "Java" },
  { value: "cpp", label: "C++" },
  { value: "c", label: "C" },
  { value: "css", label: "CSS" },
  { value: "html", label: "HTML" },
  { value: "json", label: "JSON" },
  { value: "yaml", label: "YAML" },
  { value: "markdown", label: "Markdown" },
  { value: "bash", label: "Bash" },
  { value: "sql", label: "SQL" },
  { value: "xml", label: "XML" },
];

export function CodeBlockView({ node, updateAttributes }: NodeViewProps) {
  const { t } = useTranslation();
  const language = (node.attrs.language as string) || "";

  return (
    <NodeViewWrapper as="pre">
      <select
        contentEditable={false}
        className="code-block-language"
        value={language}
        onChange={(e) => updateAttributes({ language: e.target.value })}
      >
        {CODE_LANGUAGES.map((lang) => (
          <option key={lang.value} value={lang.value}>
            {lang.labelKey ? t(lang.labelKey) : lang.label}
          </option>
        ))}
      </select>
      {/* @ts-expect-error -- NodeViewContent 'as' prop accepts any HTML tag but types only allow 'div' */}
      <NodeViewContent as="code" className={language ? `language-${language} hljs` : ""} />
    </NodeViewWrapper>
  );
}

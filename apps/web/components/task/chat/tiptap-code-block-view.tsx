'use client';

import { NodeViewContent, NodeViewWrapper, type NodeViewProps } from '@tiptap/react';

const CODE_LANGUAGES = [
  { value: '', label: 'Plain' },
  { value: 'javascript', label: 'JavaScript' },
  { value: 'typescript', label: 'TypeScript' },
  { value: 'python', label: 'Python' },
  { value: 'go', label: 'Go' },
  { value: 'rust', label: 'Rust' },
  { value: 'java', label: 'Java' },
  { value: 'cpp', label: 'C++' },
  { value: 'c', label: 'C' },
  { value: 'css', label: 'CSS' },
  { value: 'html', label: 'HTML' },
  { value: 'json', label: 'JSON' },
  { value: 'yaml', label: 'YAML' },
  { value: 'markdown', label: 'Markdown' },
  { value: 'bash', label: 'Bash' },
  { value: 'sql', label: 'SQL' },
  { value: 'xml', label: 'XML' },
];

export function CodeBlockView({ node, updateAttributes }: NodeViewProps) {
  const language = (node.attrs.language as string) || '';

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
            {lang.label}
          </option>
        ))}
      </select>
      {/* @ts-expect-error -- NodeViewContent 'as' prop accepts any HTML tag but types only allow 'div' */}
      <NodeViewContent as="code" className={language ? `language-${language} hljs` : ''} />
    </NodeViewWrapper>
  );
}

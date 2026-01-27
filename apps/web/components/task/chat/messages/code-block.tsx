'use client';

import { useState } from 'react';
import { useTheme } from 'next-themes';
import CodeMirror from '@uiw/react-codemirror';
import { javascript } from '@codemirror/lang-javascript';
import { python } from '@codemirror/lang-python';
import { go } from '@codemirror/lang-go';
import { rust } from '@codemirror/lang-rust';
import { java } from '@codemirror/lang-java';
import { cpp } from '@codemirror/lang-cpp';
import { css } from '@codemirror/lang-css';
import { html } from '@codemirror/lang-html';
import { json } from '@codemirror/lang-json';
import { markdown } from '@codemirror/lang-markdown';
import { yaml } from '@codemirror/lang-yaml';
import { vscodeDark, vscodeLight } from '@uiw/codemirror-theme-vscode';
import { EditorView } from '@codemirror/view';
import { IconCheck, IconCopy } from '@tabler/icons-react';
import type { Extension } from '@codemirror/state';
import { cn } from '@/lib/utils';

type CodeBlockProps = {
  children: React.ReactNode;
  className?: string;
};

const getLanguageExtension = (language?: string): Extension | undefined => {
  if (!language) return undefined;

  const lang = language.replace('language-', '').toLowerCase();

  switch (lang) {
    case 'javascript':
    case 'js':
      return javascript();
    case 'jsx':
      return javascript({ jsx: true });
    case 'typescript':
    case 'ts':
      return javascript({ typescript: true });
    case 'tsx':
      return javascript({ jsx: true, typescript: true });
    case 'python':
    case 'py':
      return python();
    case 'go':
    case 'golang':
      return go();
    case 'rust':
    case 'rs':
      return rust();
    case 'java':
      return java();
    case 'cpp':
    case 'c++':
    case 'c':
      return cpp();
    case 'css':
    case 'scss':
    case 'less':
      return css();
    case 'html':
    case 'htm':
      return html();
    case 'json':
      return json();
    case 'markdown':
    case 'md':
      return markdown();
    case 'yaml':
    case 'yml':
      return yaml();
    case 'bash':
    case 'sh':
    case 'shell':
      // Bash doesn't have a specific extension, fallback to basic
      return undefined;
    default:
      return undefined;
  }
};

export function CodeBlock({ children, className }: CodeBlockProps) {
  const [copied, setCopied] = useState(false);
  const { theme, systemTheme } = useTheme();
  const effectiveTheme = theme === 'system' ? systemTheme : theme;

  const code = String(children).replace(/\n$/, '');
  const languageExtension = getLanguageExtension(className);

  // Custom padding theme
  const paddingTheme = EditorView.theme({
    '&': {
      padding: '2px 2px',
    },
  });

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

  return (
    <div className="relative group my-4 w-fit max-w-full min-w-[50%]">
      {/* Copy button */}
      <button
        onClick={handleCopy}
        className={cn(
          'absolute top-2 right-2 z-10',
          'p-1.5 rounded-md',
          'bg-white/10 hover:bg-white/20',
          'transition-all duration-200',
          'opacity-0 group-hover:opacity-100',
          'cursor-pointer',
        )}
        title="Copy code"
      >
        {copied ? (
          <IconCheck className="h-3 w-3 text-green-400" />
        ) : (
          <IconCopy className="h-3 w-3 text-gray-400" />
        )}
      </button>

      {/* Code editor */}
      <CodeMirror
        value={code}
        theme={effectiveTheme === 'dark' ? vscodeDark : vscodeLight}
        extensions={languageExtension ? [languageExtension, EditorView.lineWrapping, paddingTheme] : [EditorView.lineWrapping, paddingTheme]}
        editable={false}
        basicSetup={{
          lineNumbers: false,
          highlightActiveLineGutter: false,
          highlightActiveLine: false,
          foldGutter: false,
        }}
        className="text-xs rounded-md overflow-hidden"
      />
    </div>
  );
}

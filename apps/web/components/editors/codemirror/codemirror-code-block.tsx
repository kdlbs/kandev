"use client";

import { useTheme } from "next-themes";
import CodeMirror from "@uiw/react-codemirror";
import { vscodeDark, vscodeLight } from "@uiw/codemirror-theme-vscode";
import { EditorView } from "@codemirror/view";
import { IconCheck, IconCopy } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { getCodeMirrorExtension } from "@/lib/languages";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";

type CodeBlockProps = {
  children: React.ReactNode;
  className?: string;
};

const getLanguageExtension = (language?: string) => {
  if (!language) return undefined;
  const lang = language.replace("language-", "").toLowerCase();
  return getCodeMirrorExtension(lang);
};

export function CodeMirrorCodeBlock({ children, className }: CodeBlockProps) {
  const { copied, copy } = useCopyToClipboard();
  const { theme, systemTheme } = useTheme();
  const effectiveTheme = theme === "system" ? systemTheme : theme;

  const code = String(children).replace(/\n$/, "");
  const languageExtension = getLanguageExtension(className);

  // Custom padding theme
  const paddingTheme = EditorView.theme({
    "&": {
      padding: "2px 2px",
    },
  });

  const handleCopy = async () => {
    await copy(code);
  };

  return (
    <div className="relative group/code-block my-4 w-fit max-w-full min-w-[50%]">
      {/* Copy button */}
      <button
        onClick={handleCopy}
        className={cn(
          "absolute top-2 right-2 z-10",
          "p-1.5 rounded-md",
          "bg-white/10 hover:bg-white/20",
          "transition-all duration-200",
          "opacity-0 group-hover/code-block:opacity-100",
          "cursor-pointer",
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
        theme={effectiveTheme === "dark" ? vscodeDark : vscodeLight}
        extensions={
          languageExtension
            ? [languageExtension, EditorView.lineWrapping, paddingTheme]
            : [EditorView.lineWrapping, paddingTheme]
        }
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

"use client";

import { useEffect, useState, useRef } from "react";
import { useTheme } from "next-themes";
import { IconCheck, IconCopy } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { highlightCode } from "./shiki-highlighter";

type ShikiCodeBlockProps = {
  children: React.ReactNode;
  className?: string;
};

export function ShikiCodeBlock({ children, className }: ShikiCodeBlockProps) {
  const { copied, copy } = useCopyToClipboard();
  const { theme, systemTheme } = useTheme();
  const effectiveTheme = theme === "system" ? systemTheme : theme;
  const isDark = effectiveTheme === "dark";

  const code = String(children).replace(/\n$/, "");
  const lang = className?.replace("language-", "").toLowerCase() ?? "";
  const [html, setHtml] = useState<string | null>(null);
  const prevKeyRef = useRef("");

  useEffect(() => {
    const key = `${lang}:${isDark}:${code}`;
    if (key === prevKeyRef.current) return;
    prevKeyRef.current = key;
    highlightCode(code, lang, isDark).then(setHtml);
  }, [code, lang, isDark]);

  return (
    <div className="relative group/code-block my-4 w-fit max-w-full min-w-[50%]">
      <button
        onClick={() => copy(code)}
        className={cn(
          "absolute top-1 right-2 z-10",
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

      {html ? (
        <div
          className="shiki-code-block rounded-md overflow-hidden text-xs"
          dangerouslySetInnerHTML={{ __html: html }}
        />
      ) : (
        <pre className="rounded-md overflow-hidden text-xs bg-background p-2 text-foreground">
          <code>{code}</code>
        </pre>
      )}
    </div>
  );
}

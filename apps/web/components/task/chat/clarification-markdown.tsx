"use client";

import ReactMarkdown, { defaultUrlTransform, type Components } from "react-markdown";
import remarkBreaks from "remark-breaks";
import remarkGfm from "remark-gfm";
import { cn } from "@/lib/utils";

const INLINE_ELEMENTS = ["strong", "em", "del", "code", "a", "br"];
const BLOCK_ELEMENTS = ["p", "ul", "ol", "li", ...INLINE_ELEMENTS];

type ClarificationMarkdownProps = {
  children: string;
  variant: "block" | "inline";
  linkBehavior?: "interactive" | "passive";
  className?: string;
};

function markdownComponents(linkBehavior: "interactive" | "passive"): Components {
  return {
    p: ({ children }) => <p className="leading-relaxed [&+p]:mt-2">{children}</p>,
    ul: ({ children }) => (
      <ul className="my-2 list-disc space-y-0.5 pl-5 marker:text-muted-foreground">{children}</ul>
    ),
    ol: ({ children }) => (
      <ol className="my-2 list-decimal space-y-0.5 pl-5 marker:text-muted-foreground">
        {children}
      </ol>
    ),
    li: ({ children }) => <li className="pl-0.5 leading-relaxed">{children}</li>,
    code: ({ children, className }) => {
      const isBlock = className?.startsWith("language-") || String(children).includes("\n");
      if (isBlock) return <span className="whitespace-pre-wrap">{children}</span>;
      return (
        <code className="whitespace-pre-wrap break-words rounded border border-foreground/10 bg-muted px-1 py-0.5 font-mono text-[0.9em] text-foreground">
          {children}
        </code>
      );
    },
    a: ({ children, href }) => {
      const isWebLink = href ? /^https?:\/\//i.test(href) : false;
      if (linkBehavior === "passive" || !isWebLink) {
        return <span className="text-primary underline underline-offset-2">{children}</span>;
      }
      return (
        <a
          href={href}
          target="_blank"
          rel="noopener noreferrer"
          className="break-words text-primary underline underline-offset-2"
        >
          {children}
        </a>
      );
    },
  };
}

export function ClarificationMarkdown({
  children,
  variant,
  linkBehavior = "interactive",
  className,
}: ClarificationMarkdownProps) {
  const content = (
    <ReactMarkdown
      allowedElements={variant === "block" ? BLOCK_ELEMENTS : INLINE_ELEMENTS}
      components={markdownComponents(linkBehavior)}
      remarkPlugins={[remarkGfm, remarkBreaks]}
      skipHtml
      unwrapDisallowed
      urlTransform={defaultUrlTransform}
    >
      {children}
    </ReactMarkdown>
  );

  if (variant === "inline") {
    return <span className={cn("min-w-0 [overflow-wrap:anywhere]", className)}>{content}</span>;
  }

  return <div className={cn("min-w-0 [overflow-wrap:anywhere]", className)}>{content}</div>;
}

export type { ClarificationMarkdownProps };

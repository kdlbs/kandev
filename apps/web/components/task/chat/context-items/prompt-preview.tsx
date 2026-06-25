"use client";

import { useCustomPrompts } from "@/hooks/domains/settings/use-custom-prompts";

type PromptPreviewProps = {
  content: string | null;
};

export function PromptPreview({ content }: PromptPreviewProps) {
  if (!content) {
    return <div className="text-xs text-muted-foreground">Custom prompt</div>;
  }

  const truncated = content.length > 2000 ? content.slice(0, 2000) + "..." : content;

  return (
    <div className="space-y-1.5">
      <div className="text-muted-foreground text-xs font-medium">Prompt</div>
      <pre className="text-[10px] leading-tight font-mono whitespace-pre-wrap break-all">
        {truncated}
      </pre>
    </div>
  );
}

type PromptPreviewFromStoreProps = {
  promptId: string;
};

export function PromptPreviewFromStore({ promptId }: PromptPreviewFromStoreProps) {
  const { prompts } = useCustomPrompts();
  const content = prompts.find((p) => p.id === promptId)?.content ?? null;

  return <PromptPreview content={content} />;
}

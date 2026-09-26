"use client";

import { IconChartBar } from "@tabler/icons-react";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { KandevRow } from "../shared";
import type { KandevRenderer } from "../types";
import { ChartBlock } from "./chart-block";
import { FilePreviewBlock } from "./file-preview-block";
import { MetricsBlock } from "./metrics-block";
import { parseRichOutput } from "./parse";
import type { RichOutputBlock } from "./types";

function RichOutputBlockView({
  block,
  sessionId,
  onOpenFile,
}: {
  block: RichOutputBlock;
  sessionId?: string;
  onOpenFile?: (path: string, repo?: string) => void;
}) {
  if (block.type === "metrics") return <MetricsBlock block={block} />;
  if (block.type === "chart") return <ChartBlock block={block} />;
  return <FilePreviewBlock block={block} sessionId={sessionId} onOpenFile={onOpenFile} />;
}

export const RichOutputRenderer: KandevRenderer = ({
  args,
  result,
  status,
  sessionId,
  onOpenFile,
}) => {
  const { t } = useTranslation();
  const output = useMemo(() => parseRichOutput(args, result), [args, result]);

  if (status !== "complete") {
    return (
      <KandevRow
        Icon={IconChartBar}
        title={t("task:richOutputTool")}
        status={status}
        hasExpandableContent={false}
      />
    );
  }

  return (
    <section
      className="my-2 max-w-full overflow-hidden rounded-xl border border-border/60 bg-background/80 px-4 py-3.5"
      data-testid="rich-output"
      aria-label={output?.title || t("task:richOutputTool")}
    >
      {output ? (
        <>
          <header className="space-y-1">
            <h3 className="text-sm font-medium tracking-tight text-foreground">{output.title}</h3>
            {output.description && (
              <p className="text-xs leading-relaxed text-muted-foreground">{output.description}</p>
            )}
          </header>
          <div className="mt-4 divide-y divide-border/50 [&>*]:py-4 [&>*:first-child]:pt-0 [&>*:last-child]:pb-0">
            {output.blocks.map((block, index) => (
              <div className="min-w-0" key={`${block.type}-${index}`}>
                <RichOutputBlockView block={block} sessionId={sessionId} onOpenFile={onOpenFile} />
              </div>
            ))}
          </div>
        </>
      ) : (
        <p className="text-xs text-muted-foreground">{t("task:richOutputUnavailable")}</p>
      )}
    </section>
  );
};

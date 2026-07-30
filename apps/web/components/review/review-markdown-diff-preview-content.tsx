import { MarkdownPreviewRenderer } from "@/components/task/markdown-preview-content";
import type { ReviewMarkdownPreview } from "./review-markdown-diff-preview";

export function ReviewMarkdownDiffPreviewContent({ preview }: { preview: ReviewMarkdownPreview }) {
  return (
    <section className="px-4 py-6" data-testid="review-markdown-diff-preview">
      {preview.isPartial && (
        <p className="mb-5 text-sm text-muted-foreground">
          Showing changed Markdown fragments. Unchanged content is not included in this diff.
        </p>
      )}
      <div className="markdown-body max-w-3xl">
        {preview.fragments.map((fragment, index) => (
          <div
            key={`${index}:${fragment}`}
            className={index === 0 ? undefined : "mt-8 border-t border-border pt-6"}
          >
            {preview.fragments.length > 1 && (
              <p className="mb-4 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                Changed section {index + 1}
              </p>
            )}
            <MarkdownPreviewRenderer content={fragment} />
          </div>
        ))}
      </div>
    </section>
  );
}

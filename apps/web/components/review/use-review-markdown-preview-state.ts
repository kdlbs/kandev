import { useCallback, useState } from "react";

export function useReviewMarkdownPreviewState() {
  const [markdownPreviewFiles, setMarkdownPreviewFiles] = useState<Set<string>>(() => new Set());
  const toggleMarkdownPreview = useCallback((fileKey: string) => {
    setMarkdownPreviewFiles((current) => {
      const next = new Set(current);
      if (next.has(fileKey)) next.delete(fileKey);
      else next.add(fileKey);
      return next;
    });
  }, []);

  return { markdownPreviewFiles, toggleMarkdownPreview };
}

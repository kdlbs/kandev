"use client";

import { useCallback, useMemo, type CSSProperties, type ReactNode, type Ref } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { triggerFileDownload } from "@/lib/utils/file-download";
import { cn } from "@/lib/utils";
import { getMessagePreview, type MessagePreview } from "@/lib/utils/message-preview";

type BoundedMessagePreviewProps = {
  source: string;
  downloadSource?: string;
  fileName: string;
  renderContent: (content: string) => ReactNode;
  preview?: MessagePreview;
  previewRef?: Ref<HTMLDivElement>;
  previewTestId?: string;
  previewDataExpanded?: boolean;
  previewStyle?: CSSProperties;
  previewClassName?: string;
  className?: string;
  testId?: string;
};

export function BoundedMessagePreview({
  source,
  downloadSource = source,
  fileName,
  renderContent,
  preview,
  previewRef,
  previewTestId,
  previewDataExpanded,
  previewStyle,
  previewClassName,
  className,
  testId = "bounded-message-preview",
}: BoundedMessagePreviewProps) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  const selectedPreview = useMemo(() => preview ?? getMessagePreview(source), [preview, source]);
  const download = useCallback(() => {
    triggerFileDownload({ fileName, content: downloadSource, isBinary: false });
  }, [downloadSource, fileName]);

  return (
    <div data-testid={testId} className={cn("space-y-1", className)}>
      <div
        ref={previewRef}
        data-testid={previewTestId}
        data-expanded={previewDataExpanded === undefined ? undefined : String(previewDataExpanded)}
        style={previewStyle}
        className={previewClassName}
      >
        {renderContent(selectedPreview.content)}
      </div>
      {selectedPreview.truncated && (
        <div
          data-testid={`${testId}-notice`}
          className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground"
        >
          <span>{t("task:messagePreviewShortened")}</span>
          <Button
            type="button"
            variant="link"
            onClick={download}
            aria-label={t("task:downloadFullMessage")}
            data-testid={`${testId}-download`}
            className={cn(
              "shrink-0 px-0 text-xs",
              isFinePointer ? "h-7" : "h-11 w-full justify-start",
            )}
          >
            {t("task:downloadFullMessage")}
          </Button>
        </div>
      )}
    </div>
  );
}

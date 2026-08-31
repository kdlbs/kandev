import { useEffect, useState } from "react";
import { IconCheck, IconCopy } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";

const COPIED_FEEDBACK_DURATION_MS = 1500;

type ChangeRequestCopyKind = "changeRequest" | "comment";

const labels: Record<ChangeRequestCopyKind, { copy: string; copied: string }> = {
  changeRequest: {
    copy: "integrations:copyChangeRequestUrl",
    copied: "integrations:changeRequestUrlCopied",
  },
  comment: {
    copy: "integrations:copyCommentUrl",
    copied: "integrations:commentUrlCopied",
  },
};

export function ChangeRequestDetailCopyButton({
  url,
  kind,
  testId,
}: {
  url?: string;
  kind: ChangeRequestCopyKind;
  testId: string;
}) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!copied) return;
    const timeout = window.setTimeout(() => setCopied(false), COPIED_FEEDBACK_DURATION_MS);
    return () => window.clearTimeout(timeout);
  }, [copied]);

  if (!url?.trim()) return null;

  const label = t(copied ? labels[kind].copied : labels[kind].copy);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="h-6 w-6 min-h-11 min-w-11 cursor-pointer p-0 text-muted-foreground hover:text-foreground [@media(min-width:640px)_and_(pointer:fine)]:min-h-0 [@media(min-width:640px)_and_(pointer:fine)]:min-w-0"
          onClick={() => {
            void copyToClipboard(url).then((success) => {
              if (success) setCopied(true);
            });
          }}
          aria-label={label}
          data-testid={testId}
        >
          {copied ? (
            <IconCheck className="h-3.5 w-3.5 text-green-500" />
          ) : (
            <IconCopy className="h-3.5 w-3.5" />
          )}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

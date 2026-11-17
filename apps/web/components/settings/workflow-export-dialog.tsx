"use client";

import { IconCheck, IconCopy, IconDownload } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { Textarea } from "@kandev/ui/textarea";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { useToast } from "@/components/toast-provider";
import { settingsActionClassName } from "@/components/settings/settings-control";
import { triggerFileDownload } from "@/lib/utils/file-download";

type WorkflowExportDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  content: string;
};

export function WorkflowExportDialog({
  open,
  onOpenChange,
  title,
  content,
}: WorkflowExportDialogProps) {
  const { t } = useTranslation();
  const { copied, copy } = useCopyToClipboard();
  const { toast } = useToast();

  function download() {
    try {
      // i18n-exempt: fixed download filename, not translated copy.
      triggerFileDownload({ fileName: "workflows.yaml", content, isBinary: false });
    } catch {
      toast({ title: t("task:failedToDownloadFile") });
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl max-h-[calc(100dvh-2rem)] grid-rows-[auto_minmax(0,1fr)_auto]">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <Textarea
          readOnly
          value={content}
          className="font-mono text-xs h-full min-h-0 max-h-96 overflow-y-auto"
        />
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            className={settingsActionClassName("cursor-pointer")}
          >
            {t("workflows:close")}
          </Button>
          <Button
            variant="outline"
            onClick={download}
            disabled={!content.trim()}
            className={settingsActionClassName("cursor-pointer")}
          >
            <IconDownload className="h-4 w-4 mr-2" />
            {t("task:download")}
          </Button>
          <Button
            onClick={() => copy(content)}
            className={settingsActionClassName("cursor-pointer")}
          >
            {copied ? (
              <IconCheck className="h-4 w-4 mr-2" />
            ) : (
              <IconCopy className="h-4 w-4 mr-2" />
            )}
            {copied ? t("workflows:copied") : t("workflows:copy")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

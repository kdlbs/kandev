import { IconDownload } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";
import { triggerFileDownload } from "@/lib/utils/file-download";

export function PlanDownloadButton({ draftContent }: { draftContent: string }) {
  const { t } = useTranslation("task");
  const { toast } = useToast();
  const label = t("task:download");

  function download() {
    try {
      // i18n-exempt: fixed download filename, not user-facing copy.
      triggerFileDownload({ fileName: "plan.md", content: draftContent, isBinary: false });
    } catch {
      toast({ title: t("task:failedToDownloadFile") });
    }
  }

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label={label}
      title={label}
      disabled={!draftContent.trim()}
      onClick={download}
    >
      <IconDownload aria-hidden="true" />
    </Button>
  );
}

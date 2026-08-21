"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconDownload } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { toast } from "@/lib/toast/sonner";
import { exportAutomationsZip } from "@/lib/api/domains/automations-export-api";
import { triggerBlobDownload } from "@/lib/utils/file-download";

// i18n-exempt: fixed download file name, not user-facing copy.
const EXPORT_FILE_NAME = "kandev-automations.zip";

type AutomationsExportButtonProps = {
  workspaceId: string;
};

/**
 * AC-37: fetches the workspace's automations as a zip and hands it to the
 * browser via an object-URL download. Deliberately does not use `window.open`
 * or an anchor `href` navigation to the export endpoint directly — that would
 * give no completion signal to disable on and no access to the response
 * status to detect a failure with.
 */
export function AutomationsExportButton({ workspaceId }: AutomationsExportButtonProps) {
  const { t } = useTranslation();
  const [downloading, setDownloading] = useState(false);

  const handleExport = async () => {
    setDownloading(true);
    try {
      const blob = await exportAutomationsZip(workspaceId);
      triggerBlobDownload(blob, EXPORT_FILE_NAME);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t("automations:exportFailed"));
    } finally {
      setDownloading(false);
    }
  };

  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      data-testid="export-automations-button"
      className="min-h-11 cursor-pointer md:min-h-7"
      onClick={() => void handleExport()}
      disabled={downloading}
    >
      <IconDownload className="h-4 w-4 mr-2" />
      {t("automations:exportZip")}
    </Button>
  );
}

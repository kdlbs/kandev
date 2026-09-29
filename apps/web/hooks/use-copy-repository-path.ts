import { useCallback } from "react";
import { useToast } from "@/components/toast-provider";
import { copyPathToClipboard } from "@/lib/utils/copy-repository-path";
import { useTranslation } from "react-i18next";

export function useCopyRepositoryPath() {
  const { t } = useTranslation();
  const { toast } = useToast();

  return useCallback(
    async (filePath: string) => {
      if ((await copyPathToClipboard(filePath)) === "unsafe") {
        toast({ description: t("task:copyPathWithControlCharacters") });
      }
    },
    [t, toast],
  );
}

import { useCallback } from "react";
import { useToast } from "@/components/toast-provider";
import { formatBytes } from "@/lib/utils/format-bytes";
import { MAX_FILES, MAX_FILE_SIZE, MAX_TOTAL_SIZE } from "./file-attachment";
import { useTranslation } from "react-i18next";

export function useAttachmentCountFeedback() {
  const { t } = useTranslation();
  const { toast } = useToast();

  return useCallback(() => {
    toast({
      title: t("task:attachmentLimitReached"),
      description: t("task:youCanAttachUpToFiles", { maxFiles: MAX_FILES }),
      variant: "error",
    });
  }, [toast]);
}

export function useAttachmentTotalSizeFeedback() {
  const { t } = useTranslation();
  const { toast } = useToast();

  return useCallback(() => {
    toast({
      title: t("task:attachmentLimitReached"),
      description: t("task:attachmentsCanTotalUpTo", { bytes: formatBytes(MAX_TOTAL_SIZE) }),
      variant: "error",
    });
  }, [toast]);
}

export function useAttachmentFileFeedback() {
  const { t } = useTranslation();
  const { toast } = useToast();

  return useCallback(
    (file: File): boolean => {
      if (file.size <= MAX_FILE_SIZE) return false;

      toast({
        title: t("task:attachmentIsTooLarge"),
        description: t("task:isTheMaximumFileSizeIs", {
          name: file.name,
          size: formatBytes(file.size),
          maxSize: formatBytes(MAX_FILE_SIZE),
        }),
        variant: "error",
      });
      return true;
    },
    [toast],
  );
}

export function useUnreadablePastedImageFeedback() {
  const { t } = useTranslation();
  const { toast } = useToast();

  return useCallback(() => {
    toast({
      title: t("task:pastedImageCouldnTBeAttached"),
      description: t("task:theBrowserDidnTProvideImage"),
      variant: "error",
    });
  }, [toast]);
}

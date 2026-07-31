import { useCallback } from "react";
import { useToast } from "@/components/toast-provider";
import { formatBytes } from "@/lib/utils/format-bytes";
import { MAX_FILES, MAX_FILE_SIZE, MAX_TOTAL_SIZE } from "./file-attachment";

export function useAttachmentCountFeedback() {
  const { toast } = useToast();

  return useCallback(() => {
    toast({
      title: "Attachment limit reached",
      description: `You can attach up to ${MAX_FILES} files.`,
      variant: "error",
    });
  }, [toast]);
}

export function useAttachmentTotalSizeFeedback() {
  const { toast } = useToast();

  return useCallback(() => {
    toast({
      title: "Attachment limit reached",
      description: `Attachments can total up to ${formatBytes(MAX_TOTAL_SIZE)}.`,
      variant: "error",
    });
  }, [toast]);
}

export function useAttachmentFileFeedback() {
  const { toast } = useToast();

  return useCallback(
    (file: File): boolean => {
      if (file.size <= MAX_FILE_SIZE) return false;

      toast({
        title: "Attachment is too large",
        description: `${file.name} is ${formatBytes(file.size)}. The maximum file size is ${formatBytes(MAX_FILE_SIZE)}.`,
        variant: "error",
      });
      return true;
    },
    [toast],
  );
}

export function useUnreadablePastedImageFeedback() {
  const { toast } = useToast();

  return useCallback(() => {
    toast({
      title: "Pasted image couldn’t be attached",
      description:
        "The browser didn’t provide image data. Save the image, then attach the file instead.",
      variant: "error",
    });
  }, [toast]);
}

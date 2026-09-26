"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Dialog, DialogContent } from "@kandev/ui/dialog";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTranslation } from "react-i18next";
import type { ConversationForkFormContext } from "./conversation-fork-types";
import { ConversationForkChip } from "./conversation-fork-chip";
import { ConversationForkPreview } from "./conversation-fork-preview";

export function ConversationForkControls({
  fork,
  modelId,
}: {
  fork: ConversationForkFormContext;
  modelId?: string;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const [previewOpen, setPreviewOpen] = useState(false);
  const lastEstimateKey = useRef("");

  useEffect(() => {
    const model = modelId?.trim() ?? "";
    const estimateKey = `${fork.snapshot.descriptor.id}:${model}`;
    if (!model || estimateKey === lastEstimateKey.current) return;
    lastEstimateKey.current = estimateKey;
    fork.onModelChange(model);
  }, [fork.onModelChange, fork.snapshot.descriptor.id, modelId]);

  const previewContext = useMemo(
    () => ({ ...fork, onPreview: () => setPreviewOpen(true) }),
    [fork],
  );
  const closePreview = () => setPreviewOpen(false);

  return (
    <>
      <ConversationForkChip fork={previewContext} />
      {isMobile ? (
        <Drawer
          open={previewOpen}
          onOpenChange={(open) => {
            if (!open) closePreview();
          }}
        >
          <DrawerContent
            className="!top-0 !bottom-auto !mt-0 !h-dvh !max-h-dvh min-h-0 overflow-hidden rounded-none pb-[env(safe-area-inset-bottom,0px)]"
            data-testid="conversation-fork-preview-overlay"
          >
            <DrawerHeader className="sr-only">
              <DrawerTitle>{t("task:conversationForkPreview")}</DrawerTitle>
            </DrawerHeader>
            {previewOpen && <ConversationForkPreview fork={previewContext} onBack={closePreview} />}
          </DrawerContent>
        </Drawer>
      ) : (
        <Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
          <DialogContent
            className="flex h-[80dvh] max-h-[80dvh] min-h-0 flex-col overflow-hidden p-0 sm:max-w-lg"
            data-testid="conversation-fork-preview-overlay"
          >
            {previewOpen && <ConversationForkPreview fork={previewContext} onBack={closePreview} />}
          </DialogContent>
        </Dialog>
      )}
    </>
  );
}

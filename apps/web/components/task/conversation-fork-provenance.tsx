"use client";

import { useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { IconGitFork } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import type { ReactNode } from "react";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import {
  getConversationForkContent,
  getConversationForkDraft,
} from "@/lib/api/domains/conversation-fork-api";
import type {
  ConversationForkContent,
  ConversationForkDescriptor,
} from "@/lib/api/domains/conversation-fork-api";

function ForkProvenanceTrigger({ open, onOpen }: { open: boolean; onOpen: () => void }) {
  const { t } = useTranslation();
  return (
    <Button
      type="button"
      variant="outline"
      onClick={onOpen}
      className="min-h-11 gap-1 rounded-full px-2.5 py-0.5 text-[10px]"
      aria-haspopup="dialog"
      aria-expanded={open}
      data-testid="conversation-fork-provenance-trigger"
    >
      <IconGitFork className="size-3" aria-hidden="true" />
      {t("task:conversationForkProvenance")}
    </Button>
  );
}

function ForkProvenanceDetails({
  descriptor,
  content,
  loading,
  failed,
}: {
  descriptor: ConversationForkDescriptor | null;
  content: ConversationForkContent | null;
  loading: boolean;
  failed: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="grid min-h-0 gap-3">
      <p className="text-sm text-muted-foreground">
        {t("task:conversationForkSourceProvenance", {
          title: descriptor?.source_task_title ?? t("task:conversationForkSourceTask"),
          count: descriptor?.message_count ?? 0,
        })}
      </p>
      {loading && (
        <p role="status" className="text-sm text-muted-foreground">
          {t("task:conversationForkPreparing")}
        </p>
      )}
      {failed && (
        <p role="alert" className="text-sm text-destructive">
          {t("task:conversationForkProvenanceUnavailable")}
        </p>
      )}
      {content && (
        <pre
          className="max-h-[60dvh] min-h-0 overflow-auto whitespace-pre-wrap break-words rounded-md border bg-background p-3 text-xs leading-relaxed"
          data-testid="conversation-fork-provenance-content"
        >
          {content.content}
        </pre>
      )}
    </div>
  );
}

function ForkProvenanceSurface({
  open,
  onOpenChange,
  descriptor,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  descriptor: ConversationForkDescriptor | null;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  if (usesTouchDrawer) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent className="max-h-[85dvh] pb-[env(safe-area-inset-bottom,0px)]">
          <DrawerHeader>
            <DrawerTitle>{t("task:conversationForkProvenance")}</DrawerTitle>
            <DrawerDescription>
              {descriptor?.source_task_title ?? t("task:conversationForkSourceTask")}
            </DrawerDescription>
          </DrawerHeader>
          <div className="min-h-0 overflow-y-auto px-4 pb-4">{children}</div>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85dvh] min-h-0 flex-col overflow-hidden sm:max-w-2xl">
        <DialogHeader className="shrink-0">
          <DialogTitle>{t("task:conversationForkProvenance")}</DialogTitle>
        </DialogHeader>
        {children}
      </DialogContent>
    </Dialog>
  );
}

export function ConversationForkProvenance({ forkId }: { forkId: string }) {
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [descriptor, setDescriptor] = useState<ConversationForkDescriptor | null>(null);
  const [content, setContent] = useState<ConversationForkContent | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    if (!open || content) return;
    let current = true;
    setLoading(true);
    setFailed(false);
    void Promise.all([getConversationForkDraft(forkId), getConversationForkContent(forkId)])
      .then(([nextDescriptor, nextContent]) => {
        if (!current) return;
        setDescriptor(nextDescriptor);
        setContent(nextContent);
      })
      .catch(() => {
        if (current) setFailed(true);
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    return () => {
      current = false;
    };
  }, [content, forkId, open]);

  return (
    <>
      <ForkProvenanceTrigger open={open} onOpen={() => setOpen(true)} />
      <ForkProvenanceSurface open={open} onOpenChange={setOpen} descriptor={descriptor}>
        <ForkProvenanceDetails
          descriptor={descriptor}
          content={content}
          loading={loading}
          failed={failed}
        />
      </ForkProvenanceSurface>
    </>
  );
}

"use client";

import { useCallback, useEffect, useState } from "react";
import { IconAlertTriangle, IconCopy, IconExternalLink } from "@tabler/icons-react";

import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";

import {
  createShare,
  previewShare,
  type Share,
  type SnapshotPreview,
} from "@/lib/api/domains/share-api";
import { useShares } from "@/hooks/domains/session/use-shares";

import { ShareList } from "./share-list";
import { ShareSnapshotPreview } from "./share-snapshot-preview";

type DialogState =
  | { kind: "loading" }
  | { kind: "preview"; snapshot: SnapshotPreview }
  | { kind: "publishing"; snapshot: SnapshotPreview }
  | { kind: "published"; share: Share }
  | { kind: "error"; message: string };

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskId: string;
  sessionId: string;
};

export function ShareDialog({ open, onOpenChange, taskId, sessionId }: Props) {
  const [state, setState] = useState<DialogState>({ kind: "loading" });
  const { shares, refresh } = useShares(open ? taskId : null, open ? sessionId : null);

  const loadPreview = useCallback(async () => {
    setState({ kind: "loading" });
    try {
      const snapshot = await previewShare(taskId, sessionId);
      setState({ kind: "preview", snapshot });
    } catch (e) {
      setState({
        kind: "error",
        message: e instanceof Error ? e.message : "Failed to load preview.",
      });
    }
  }, [taskId, sessionId]);

  useEffect(() => {
    if (!open) return;
    // Defer to a microtask so setState runs outside the synchronous effect
    // body (react-hooks/set-state-in-effect).
    void Promise.resolve().then(() => loadPreview());
  }, [open, loadPreview]);

  const handlePublish = useCallback(
    async (snapshot: SnapshotPreview) => {
      setState({ kind: "publishing", snapshot });
      try {
        const share = await createShare(taskId, sessionId);
        setState({ kind: "published", share });
        void refresh();
      } catch (e) {
        setState({
          kind: "error",
          message: e instanceof Error ? e.message : "Failed to publish share.",
        });
      }
    },
    [taskId, sessionId, refresh],
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] w-full max-w-[min(640px,92vw)] flex-col gap-3 overflow-hidden sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Share this task</DialogTitle>
          <DialogDescription>
            Publish a redacted snapshot of this completed task as a secret GitHub Gist on your
            account.
          </DialogDescription>
        </DialogHeader>
        <DialogBody
          state={state}
          shares={shares}
          onPublish={handlePublish}
          onRevoked={refresh}
          onRetry={loadPreview}
          onClose={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  );
}

type BodyProps = {
  state: DialogState;
  shares: Share[];
  onPublish: (snapshot: SnapshotPreview) => void;
  onRevoked: () => void;
  onRetry: () => void;
  onClose: () => void;
};

function DialogBody({ state, shares, onPublish, onRevoked, onRetry, onClose }: BodyProps) {
  if (state.kind === "loading") {
    return <p className="text-sm text-muted-foreground">Building preview…</p>;
  }
  if (state.kind === "error") {
    return (
      <div className="flex flex-col gap-2">
        <Alert variant="destructive">
          <AlertDescription>{state.message}</AlertDescription>
        </Alert>
        <DialogFooter>
          <Button variant="outline" onClick={onClose} className="cursor-pointer">
            Close
          </Button>
          <Button onClick={onRetry} className="cursor-pointer">
            Try again
          </Button>
        </DialogFooter>
      </div>
    );
  }
  if (state.kind === "published") {
    return (
      <PublishedState share={state.share} shares={shares} onRevoked={onRevoked} onClose={onClose} />
    );
  }
  const snapshot = state.snapshot;
  const isPublishing = state.kind === "publishing";
  return (
    <div className="flex min-w-0 flex-1 flex-col gap-3 overflow-hidden">
      <Alert>
        <IconAlertTriangle className="h-4 w-4" />
        <AlertDescription>
          Anyone with this link can view this conversation. Review the preview carefully before
          publishing — once published, the snapshot is uploaded to GitHub Gist on your account.
        </AlertDescription>
      </Alert>
      <div className="flex min-w-0 flex-1 flex-col gap-3 overflow-y-auto pr-1">
        <ShareList shares={shares} onRevoked={onRevoked} />
        <ShareSnapshotPreview snapshot={snapshot} />
      </div>
      <DialogFooter>
        <Button
          variant="outline"
          onClick={onClose}
          disabled={isPublishing}
          className="cursor-pointer"
        >
          Cancel
        </Button>
        <Button
          onClick={() => onPublish(snapshot)}
          disabled={isPublishing}
          className="cursor-pointer"
        >
          {isPublishing ? "Publishing…" : "Publish to GitHub Gist"}
        </Button>
      </DialogFooter>
    </div>
  );
}

function PublishedState({
  share,
  shares,
  onRevoked,
  onClose,
}: {
  share: Share;
  shares: Share[];
  onRevoked: () => void;
  onClose: () => void;
}) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    await navigator.clipboard.writeText(share.url);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  return (
    <div className="flex min-w-0 flex-1 flex-col gap-3 overflow-hidden">
      <Alert>
        <AlertDescription>
          Published. Anyone with this link can view the conversation.
        </AlertDescription>
      </Alert>
      <div className="flex min-w-0 items-center gap-2 rounded border bg-muted/30 p-2">
        <a
          href={share.url}
          target="_blank"
          rel="noopener noreferrer"
          className="flex min-w-0 flex-1 items-center gap-1 text-sm text-primary hover:underline cursor-pointer"
        >
          <IconExternalLink className="h-3 w-3 flex-shrink-0" />
          <span className="min-w-0 flex-1 truncate" title={share.url}>
            {share.url}
          </span>
        </a>
        <Button
          size="sm"
          variant="outline"
          onClick={handleCopy}
          className="flex-shrink-0 cursor-pointer"
        >
          <IconCopy className="h-3 w-3" />
          {copied ? "Copied" : "Copy"}
        </Button>
      </div>
      <div className="min-w-0 flex-1 overflow-y-auto pr-1">
        <ShareList shares={shares} onRevoked={onRevoked} />
      </div>
      <DialogFooter>
        <Button onClick={onClose} className="cursor-pointer">
          Done
        </Button>
      </DialogFooter>
    </div>
  );
}

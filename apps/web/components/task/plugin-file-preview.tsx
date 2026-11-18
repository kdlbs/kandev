"use client";
import Link from "@/components/routing/app-link";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerTitle,
  DrawerDescription,
  DrawerFooter,
} from "@kandev/ui/drawer";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { usePluginFilePreview, type PluginPreviewInput } from "@/hooks/use-plugin-file-preview";
import { usePluginRegistry } from "@/lib/plugins/registry";
import { fileViewerCandidates } from "@/lib/plugins/file-viewer-selection";
import { useAppStore } from "@/components/state-provider";
import { PluginErrorBoundary } from "@/components/plugins/plugin-error-boundary";
import { useFileViewerChoice } from "@/hooks/use-file-viewer-choice";
import { FilePreviewToolbar } from "./file-preview-toolbar";
import { usePreviewAsText } from "@/hooks/use-preview-as-text";
import { FileViewerContent } from "./file-viewer-content";
import { FindFileViewer } from "./find-file-viewer";
import { UnavailableFileViewers } from "./unavailable-file-viewers";

export function PluginFilePreview({
  path,
  repo,
  sessionId,
  onOpenText,
}: {
  path: string;
  repo?: string;
  sessionId: string;
  onOpenText?: () => void;
}) {
  const { t } = useTranslation();
  const text = usePreviewAsText(path, repo, sessionId);
  usePluginRegistry();
  const candidates = fileViewerCandidates(path);
  const [choice, setChoice] = useFileViewerChoice(sessionId, path, repo);
  const viewer = resolveViewerChoice(candidates, choice);
  const selectViewer = (value: string) => {
    text.clear();
    setChoice(value);
  };
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const container = useRef<HTMLDivElement>(null);
  const [visible, setVisible] = useState(false);
  const canPreview = !!viewer && !!workspaceId && visible;
  useEffect(() => {
    if (!container.current) return;
    const observer = new IntersectionObserver((entries) =>
      setVisible(entries.some((e) => e.isIntersecting)),
    );
    observer.observe(container.current);
    return () => observer.disconnect();
  }, []);
  return (
    <div ref={container} className="flex h-full min-h-0 flex-col" data-testid="plugin-file-preview">
      <FilePreviewToolbar
        path={path}
        repo={repo}
        sessionId={sessionId}
        candidates={candidates}
        choice={choice}
        viewerAvailable={!!viewer}
        text={text}
        onSelect={selectViewer}
        onOpenText={onOpenText}
      />
      {text.failed && <p role="alert">{t("plugins:viewerTextFailed")}</p>}
      {text.file ? (
        <FileViewerContent
          path={path}
          repo={repo}
          content={text.file.content}
          sessionId={sessionId}
        />
      ) : null}
      {!text.file && canPreview && (
        <SelectedPreview
          key={`${path}/${repo}/${sessionId}/${viewer!.pluginId}/${viewer!.id}/${viewer!.generation}`}
          path={path}
          repo={repo}
          sessionId={sessionId}
          workspaceId={workspaceId!}
          viewer={viewer!}
        />
      )}
      {!text.file && !canPreview && (
        <div className="flex flex-1 flex-col items-center justify-center gap-3 p-4">
          <p>
            {t(
              choice || candidates.length === 0
                ? "plugins:viewerUnavailable"
                : "plugins:viewerChoose",
            )}
          </p>
          <UnavailableFileViewers path={path} />
          <FindFileViewer path={path} repo={repo} sessionId={sessionId} />
        </div>
      )}
    </div>
  );
}

function SelectedPreview(input: PluginPreviewInput) {
  const { t } = useTranslation();
  const preview = usePluginFilePreview(input);
  const { state } = preview;
  const Component = input.viewer.Component;
  const messages = {
    loading: t("plugins:viewerLoading"),
    failed: t("plugins:viewerFailed"),
    busy: t("plugins:viewerBusy"),
    denied: t("plugins:viewerDenied"),
    "too-large": t("plugins:viewerTooLarge"),
  };
  const statusMessage =
    state.status in messages
      ? messages[state.status as keyof typeof messages]
      : t("plugins:viewerInactive");
  const failed = ["failed", "denied", "too-large"].includes(state.status);
  const fallback = (
    <p role="alert" className="p-4">
      {t("plugins:viewerFailed")}
    </p>
  );
  const bytes = state.bytes;
  const readBytes = async (signal: AbortSignal) => {
    signal.throwIfAborted();
    if (!bytes) throw new Error();
    return bytes;
  };
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {state.status === "ready" && bytes && state.selection ? (
        <PluginErrorBoundary context="file viewer" fallback={fallback}>
          <Component
            file={{
              name: input.path.replaceAll("\\", "/").split("/").pop() ?? input.path,
              sizeBytes: state.selection.size_bytes,
              generation: state.selection.id,
            }}
            readBytes={readBytes}
          />
        </PluginErrorBoundary>
      ) : (
        <div className="flex flex-1 flex-col items-center justify-center gap-3 p-4">
          <p role={failed ? "alert" : "status"}>{statusMessage}</p>
          {state.status === "denied" && (
            <Link href="/settings/plugins" className="underline">
              {t("plugins:viewerSettings")}
            </Link>
          )}
          {state.status !== "loading" && state.status !== "approval" && (
            <Button
              className="max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
              onClick={() => void preview.request()}
            >
              {t("common:retryPreview")}
            </Button>
          )}
        </div>
      )}
      {state.status === "approval" && state.selection && (
        <PreviewApproval
          preview={preview}
          plugin={input.viewer.title}
          pluginId={input.viewer.pluginId}
          task={input.sessionId}
        />
      )}
    </div>
  );
}

function PreviewApproval({
  preview,
  plugin,
  pluginId,
  task,
}: {
  preview: ReturnType<typeof usePluginFilePreview>;
  plugin: string;
  pluginId: string;
  task: string;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const selection = preview.state.selection!;
  const body = (
    <div className="min-h-0 overflow-y-auto overscroll-contain break-all p-4 text-sm">
      <p>{plugin}</p>
      <p>{pluginId}</p>
      <p>{selection.path}</p>
      {selection.original_path !== selection.path && <p>{selection.original_path}</p>}
      <p>
        {t("plugins:viewerApprovalDetails", {
          bytes: selection.size_bytes,
          execution: selection.execution_label,
          task,
        })}
      </p>
      <Link className="underline" href="/settings/plugins">
        {t("plugins:viewerSettings")}
      </Link>
    </div>
  );
  const actions = (
    <>
      <Button
        className="max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        variant="outline"
        onClick={preview.dismissApproval}
      >
        {t("common:cancel")}
      </Button>
      <Button
        className="max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        onClick={preview.approve}
      >
        {t("plugins:viewerPreviewOnce")}
      </Button>
    </>
  );
  const title = t("plugins:viewerExternalTitle");
  const description = t("plugins:viewerExternalDescription");
  return (
    <ApprovalSurface
      mobile={isMobile}
      title={title}
      description={description}
      body={body}
      actions={actions}
      cancel={preview.dismissApproval}
    />
  );
}
function ApprovalSurface({
  mobile,
  title,
  description,
  body,
  actions,
  cancel,
}: {
  mobile: boolean;
  title: string;
  description: string;
  body: ReactNode;
  actions: ReactNode;
  cancel: () => void;
}) {
  if (mobile)
    return (
      <Drawer
        open
        onOpenChange={(open) => {
          if (!open) cancel();
        }}
      >
        <DrawerContent className="max-h-[85dvh]">
          <div className="p-4">
            <DrawerTitle>{title}</DrawerTitle>
            <DrawerDescription>{description}</DrawerDescription>
          </div>
          {body}
          <DrawerFooter className="pb-[max(1rem,env(safe-area-inset-bottom))]">
            {actions}
          </DrawerFooter>
        </DrawerContent>
      </Drawer>
    );
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) cancel();
      }}
    >
      <DialogContent>
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>{description}</DialogDescription>
        {body}
        <DialogFooter>{actions}</DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function resolveViewerChoice(
  candidates: ReturnType<typeof fileViewerCandidates>,
  choice: string | null,
) {
  if (choice) return candidates.find((viewer) => `${viewer.pluginId}/${viewer.id}` === choice);
  return candidates.length === 1 ? candidates[0] : undefined;
}

import { useState } from "react";
import ReactMarkdown from "react-markdown";
import { IconMessagePlus, IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { markdownComponents, remarkPlugins } from "@/components/shared/markdown-components";

export function formatTimeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const minutes = Math.floor(diff / 60000);
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

function formatMs(ms: number): string {
  const totalSeconds = Math.floor(ms / 1000);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  if (hours > 0) return `${hours}h ${minutes}m`;
  if (minutes > 0) return `${minutes}m ${seconds}s`;
  return `${seconds}s`;
}

export function formatDuration(startedAt: string, completedAt: string): string {
  return formatMs(new Date(completedAt).getTime() - new Date(startedAt).getTime());
}

export function formatElapsed(startedAt: string): string {
  return formatMs(Date.now() - new Date(startedAt).getTime());
}

export function AuthorAvatar({ src, author }: { src: string; author: string }) {
  const inner = src ? (
    // eslint-disable-next-line @next/next/no-img-element
    <img src={src} alt={author} className="h-5 w-5 rounded-full shrink-0" />
  ) : (
    <div className="h-5 w-5 rounded-full bg-muted flex items-center justify-center shrink-0 text-[10px] font-medium text-muted-foreground">
      {author[0]?.toUpperCase() ?? "?"}
    </div>
  );
  return (
    <a
      href={`https://github.com/${author}`}
      target="_blank"
      rel="noopener noreferrer"
      className="shrink-0 cursor-pointer"
    >
      {inner}
    </a>
  );
}

export function AuthorLink({ author }: { author: string }) {
  return (
    <a
      href={`https://github.com/${author}`}
      target="_blank"
      rel="noopener noreferrer"
      className="text-xs font-medium hover:underline cursor-pointer"
    >
      {author}
    </a>
  );
}

export function CollapsibleSection({
  title,
  count,
  defaultOpen,
  subtitle,
  onAddAll,
  addAllLabel,
  children,
}: {
  title: string;
  count: number;
  defaultOpen?: boolean;
  subtitle?: React.ReactNode;
  onAddAll?: () => void;
  addAllLabel?: string;
  children: React.ReactNode;
}) {
  const [open, setOpen] = useState(defaultOpen ?? true);

  return (
    <div>
      <div className="flex items-center">
        <button
          type="button"
          className="flex items-center gap-1.5 flex-1 py-2 text-xs font-semibold text-foreground/80 hover:text-foreground cursor-pointer"
          onClick={() => setOpen(!open)}
        >
          {open ? (
            <IconChevronDown className="h-3.5 w-3.5" />
          ) : (
            <IconChevronRight className="h-3.5 w-3.5" />
          )}
          {title}
          <span className="text-muted-foreground font-normal">({count})</span>
        </button>
        {onAddAll && count > 0 && open && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="sm"
                variant="ghost"
                className="h-6 px-1.5 cursor-pointer text-[10px] text-muted-foreground hover:text-foreground gap-1"
                onClick={onAddAll}
              >
                <IconMessagePlus className="h-3 w-3" />
                Add all
              </Button>
            </TooltipTrigger>
            <TooltipContent>{addAllLabel ?? "Add all to chat context"}</TooltipContent>
          </Tooltip>
        )}
      </div>
      {subtitle && open && <div className="pb-1">{subtitle}</div>}
      {open && <div className="space-y-2 pb-3">{children}</div>}
    </div>
  );
}

export function AddToContextButton({
  onClick,
  tooltip,
}: {
  onClick: () => void;
  tooltip?: string;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="sm"
          variant="ghost"
          className="h-6 w-6 p-0 cursor-pointer shrink-0 text-muted-foreground hover:text-foreground"
          onClick={(e) => {
            e.stopPropagation();
            onClick();
          }}
        >
          <IconMessagePlus className="h-3 w-3" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{tooltip ?? "Add to chat context"}</TooltipContent>
    </Tooltip>
  );
}

export function PRMarkdownBody({ body }: { body: string }) {
  return (
    <div className="prose prose-sm dark:prose-invert max-w-none text-sm prose-p:my-1.5 prose-p:leading-relaxed prose-ul:my-1.5 prose-ol:my-1.5 prose-li:my-0.5 prose-pre:my-2 prose-headings:text-foreground prose-headings:font-bold prose-strong:text-foreground prose-strong:font-bold">
      <ReactMarkdown remarkPlugins={remarkPlugins} components={markdownComponents}>
        {body}
      </ReactMarkdown>
    </div>
  );
}

export function getTimeAgoColor(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const hours = diff / (1000 * 60 * 60);
  if (hours < 24) return "text-green-600 dark:text-green-400";
  if (hours < 48) return "text-muted-foreground";
  return "text-orange-600 dark:text-orange-400";
}

export function ExpandableBody({ body, className }: { body: string; className?: string }) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className={className}>
      <div className={expanded ? "" : "line-clamp-4"}>
        <PRMarkdownBody body={body} />
      </div>
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation();
          setExpanded(!expanded);
        }}
        className="text-[10px] text-blue-600 dark:text-blue-400 hover:underline mt-0.5 cursor-pointer"
      >
        {expanded ? "Show less" : "Show more"}
      </button>
    </div>
  );
}

export function FeedbackItemRow({
  author,
  authorAvatar,
  body,
  createdAt,
  metaBadge,
  onAddAsContext,
  isReply,
}: {
  author: string;
  authorAvatar: string;
  body?: string;
  createdAt: string;
  metaBadge?: React.ReactNode;
  onAddAsContext?: () => void;
  isReply?: boolean;
}) {
  return (
    <div className={isReply ? "ml-4 pl-2.5 border-l-2 border-border" : ""}>
      <div className="px-2.5 py-2 rounded-md border border-border bg-muted/30 space-y-1">
        <div className="flex items-center gap-2">
          <AuthorAvatar src={authorAvatar} author={author} />
          <AuthorLink author={author} />
          {metaBadge}
          <span className="text-[10px] text-muted-foreground ml-auto shrink-0">
            {formatTimeAgo(createdAt)}
          </span>
          {onAddAsContext && <AddToContextButton onClick={onAddAsContext} />}
        </div>
        {body && <ExpandableBody body={body} className="pl-7" />}
      </div>
    </div>
  );
}

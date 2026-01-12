'use client';

import { FormEvent, useEffect, useRef, useState } from 'react';
import { IconBrain, IconListCheck, IconLoader2, IconPaperclip, IconTerminal2 } from '@tabler/icons-react';
import ReactMarkdown from 'react-markdown';
import remarkBreaks from 'remark-breaks';
import remarkGfm from 'remark-gfm';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { useAppStore } from '@/components/state-provider';
import type { Comment } from '@/lib/types/http';

type AgentOption = {
  id: string;
  label: string;
};

type TaskChatPanelProps = {
  taskId: string;
  agents: AgentOption[];
  onSend: (message: string) => void;
  isLoading?: boolean;
};

export function TaskChatPanel({ taskId, agents, onSend, isLoading }: TaskChatPanelProps) {
  const [messageInput, setMessageInput] = useState('');
  const [selectedAgent, setSelectedAgent] = useState(agents[0]?.id ?? '');
  const [planModeEnabled, setPlanModeEnabled] = useState(false);
  const [isSending, setIsSending] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  const commentsState = useAppStore((state) => state.comments);
  const comments = commentsState?.items ?? [];
  const commentsLoading = commentsState?.isLoading ?? false;

  // Filter to only show message, content, and tool_call types (not progress, etc)
  const visibleComments = comments.filter(
    (c) => c.type === 'message' || c.type === 'content' || c.type === 'tool_call' || !c.type
  );

  // Scroll to bottom when new comments arrive
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [visibleComments.length]);

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault();
    const trimmed = messageInput.trim();
    if (!trimmed || isSending) return;
    setIsSending(true);
    setMessageInput('');
    try {
      await onSend(trimmed);
    } finally {
      setIsSending(false);
    }
  };

  const formatContent = (comment: Comment) => {
    // For content type comments, the content might be streamed chunks
    // For message type, show as-is
    return comment.content || '(empty)';
  };

  return (
    <>
      <div className="flex-1 min-h-0 overflow-y-auto rounded-lg bg-background p-3 space-y-3">
        {(isLoading || commentsLoading) && (
          <div className="flex items-center justify-center py-8 text-muted-foreground">
            <IconLoader2 className="h-5 w-5 animate-spin mr-2" />
            <span>Loading comments...</span>
          </div>
        )}
        {!isLoading && !commentsLoading && visibleComments.length === 0 && (
          <div className="flex items-center justify-center py-8 text-muted-foreground">
            <span>No messages yet. Start the conversation!</span>
          </div>
        )}
        {visibleComments.map((comment) => {
          // Tool call comments have a special rendering
          if (comment.type === 'tool_call') {
            const metadata = comment.metadata as { title?: string; status?: string } | undefined;
            return (
              <div
                key={comment.id}
                className="flex items-center gap-2 px-3 py-2 text-sm rounded-md bg-muted/30 border border-border/30"
              >
                <IconTerminal2 className="h-4 w-4 text-amber-600 dark:text-amber-400 flex-shrink-0" />
                <span className="font-mono text-xs text-muted-foreground">
                  {metadata?.title || comment.content}
                </span>
              </div>
            );
          }

          // Regular message or agent response - render as markdown
          const isUser = comment.author_type === 'user';
          return (
            <div
              key={comment.id}
              className={cn(
                'max-w-[85%] rounded-lg px-4 py-3 text-sm',
                isUser
                  ? 'ml-auto bg-primary text-primary-foreground'
                  : 'bg-muted text-foreground'
              )}
            >
              <p className="text-[11px] uppercase tracking-wide mb-2 opacity-70">
                {isUser ? 'You' : 'Agent'}
              </p>
              {isUser ? (
                <p className="whitespace-pre-wrap">{formatContent(comment)}</p>
              ) : (
                <div className="prose prose-sm dark:prose-invert max-w-none prose-p:my-2 prose-p:leading-relaxed prose-ul:my-2 prose-ol:my-2 prose-li:my-0.5 prose-pre:my-3 prose-code:px-1.5 prose-code:py-0.5 prose-code:bg-background/50 prose-code:rounded prose-code:text-xs prose-code:before:content-none prose-code:after:content-none prose-pre:bg-background/80 prose-pre:text-xs prose-strong:text-foreground prose-headings:text-foreground">
                  <ReactMarkdown remarkPlugins={[remarkGfm, remarkBreaks]}>
                    {formatContent(comment)}
                  </ReactMarkdown>
                </div>
              )}
            </div>
          );
        })}
        <div ref={messagesEndRef} />
      </div>
      <form onSubmit={handleSubmit} className="mt-3 flex flex-col gap-2">
        <Textarea
          value={messageInput}
          onChange={(event) => setMessageInput(event.target.value)}
          placeholder="Write to submit work to the agent..."
          className={cn(
            'min-h-[90px] resize-none',
            planModeEnabled &&
              'border-dashed border-primary/60 !bg-primary/20 dark:!bg-primary/20 shadow-[inset_0_0_0_1px_rgba(59,130,246,0.35)]'
          )}
        />
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <Select value={selectedAgent} onValueChange={setSelectedAgent}>
              <SelectTrigger className="w-[160px] cursor-pointer">
                <SelectValue placeholder="Select agent" />
              </SelectTrigger>
              <SelectContent>
                {agents.map((agent) => (
                  <SelectItem key={agent.id} value={agent.id}>
                    {agent.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <DropdownMenu>
              <Tooltip>
                <TooltipTrigger asChild>
                  <DropdownMenuTrigger asChild>
                    <Button type="button" variant="outline" size="icon" className="h-9 w-9 cursor-pointer">
                      <IconBrain className="h-4 w-4" />
                    </Button>
                  </DropdownMenuTrigger>
                </TooltipTrigger>
                <TooltipContent>Thinking level</TooltipContent>
              </Tooltip>
              <DropdownMenuContent align="start" side="top">
                <DropdownMenuItem>High</DropdownMenuItem>
                <DropdownMenuItem>Medium</DropdownMenuItem>
                <DropdownMenuItem>Low</DropdownMenuItem>
                <DropdownMenuItem>Off</DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
            <Tooltip>
              <TooltipTrigger asChild>
                <div className="flex items-center gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    className={cn(
                      'h-9 w-9 cursor-pointer',
                      planModeEnabled &&
                        'bg-primary/15 text-primary border-primary/40 shadow-[0_0_0_1px_rgba(59,130,246,0.35)]'
                    )}
                    onClick={() => setPlanModeEnabled((value) => !value)}
                  >
                    <IconListCheck className="h-4 w-4" />
                  </Button>
                  {planModeEnabled && (
                    <span className="text-xs font-medium text-primary">Plan mode active</span>
                  )}
                </div>
              </TooltipTrigger>
              <TooltipContent>Toggle plan mode</TooltipContent>
            </Tooltip>
          </div>
          <div className="flex items-center gap-2">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button type="button" variant="outline" size="icon" className="h-9 w-9 cursor-pointer">
                  <IconPaperclip className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Add attachments</TooltipContent>
            </Tooltip>
            <Button type="submit">Submit</Button>
          </div>
        </div>
      </form>
    </>
  );
}

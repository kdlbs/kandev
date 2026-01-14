'use client';

import { FormEvent, useEffect, useRef, useState } from 'react';
import {
  IconBrain,
  IconCheck,
  IconChevronDown,
  IconChevronRight,
  IconCode,
  IconEdit,
  IconEye,
  IconFile,
  IconListCheck,
  IconLoader2,
  IconPaperclip,
  IconSearch,
  IconTerminal2,
  IconX,
} from '@tabler/icons-react';
import ReactMarkdown from 'react-markdown';
import remarkBreaks from 'remark-breaks';
import remarkGfm from 'remark-gfm';
import { Button } from '@kandev/ui/button';
import { Textarea } from '@kandev/ui/textarea';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@kandev/ui/dropdown-menu';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@kandev/ui/select';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { cn } from '@/lib/utils';
import { useAppStore } from '@/components/state-provider';
import type { Comment } from '@/lib/types/http';

type AgentOption = {
  id: string;
  label: string;
};

type TaskChatPanelProps = {
  agents: AgentOption[];
  onSend: (message: string) => void;
  isLoading?: boolean;
  isAgentWorking?: boolean;
  taskDescription?: string;
};

type ToolCallMetadata = {
  tool_call_id?: string;
  tool_name?: string;
  title?: string;
  status?: 'pending' | 'running' | 'complete' | 'error';
  args?: Record<string, unknown>;
  result?: string;
};

function getToolIcon(toolName: string | undefined, className: string) {
  const name = toolName?.toLowerCase() ?? '';
  // Match ACP ToolKind values: read, edit, delete, move, search, execute
  if (name === 'edit' || name.includes('edit') || name.includes('replace') || name.includes('write') || name.includes('save')) {
    return <IconEdit className={className} />;
  }
  if (name === 'read' || name.includes('view') || name.includes('read')) {
    return <IconEye className={className} />;
  }
  if (name === 'search' || name.includes('search') || name.includes('find') || name.includes('retrieval')) {
    return <IconSearch className={className} />;
  }
  if (name === 'execute' || name.includes('terminal') || name.includes('exec') || name.includes('launch') || name.includes('process')) {
    return <IconTerminal2 className={className} />;
  }
  if (name === 'delete' || name === 'move' || name.includes('file') || name.includes('create')) {
    return <IconFile className={className} />;
  }
  return <IconCode className={className} />;
}

function getStatusIcon(status?: string) {
  switch (status) {
    case 'complete':
      return <IconCheck className="h-3.5 w-3.5 text-green-500" />;
    case 'error':
      return <IconX className="h-3.5 w-3.5 text-red-500" />;
    case 'running':
      return <IconLoader2 className="h-3.5 w-3.5 text-blue-500 animate-spin" />;
    default:
      return null;
  }
}

function ToolCallCard({ comment }: { comment: Comment }) {
  const [isExpanded, setIsExpanded] = useState(false);
  const metadata = comment.metadata as ToolCallMetadata | undefined;

  const toolName = metadata?.tool_name ?? '';
  const title = metadata?.title ?? comment.content ?? 'Tool call';
  const status = metadata?.status;
  const args = metadata?.args;
  const result = metadata?.result;

  const toolIcon = getToolIcon(toolName, 'h-4 w-4 text-amber-600 dark:text-amber-400 flex-shrink-0');
  const hasDetails = args && Object.keys(args).length > 0;

  // Extract file path from various possible sources
  let filePath: string | undefined;
  const rawPath = args?.path ?? args?.file ?? args?.file_path;
  if (typeof rawPath === 'string') {
    filePath = rawPath;
  }
  // Also try to get path from locations array if available
  if (!filePath && Array.isArray(args?.locations) && args.locations.length > 0) {
    const firstLoc = args.locations[0] as { path?: string } | undefined;
    if (firstLoc?.path) {
      filePath = firstLoc.path;
    }
  }

  return (
    <div className="rounded-md border border-border/40 bg-muted/20 overflow-hidden">
      <button
        type="button"
        onClick={() => hasDetails && setIsExpanded(!isExpanded)}
        className={cn(
          'w-full flex items-center gap-2 px-3 py-2 text-sm text-left',
          hasDetails && 'cursor-pointer hover:bg-muted/40 transition-colors'
        )}
        disabled={!hasDetails}
      >
        {toolIcon}
        <span className="flex-1 font-mono text-xs text-muted-foreground truncate">
          {title}
        </span>
        {filePath && (
          <span className="text-xs text-muted-foreground/60 truncate max-w-[200px]">
            {filePath}
          </span>
        )}
        {getStatusIcon(status)}
        {hasDetails && (
          isExpanded
            ? <IconChevronDown className="h-4 w-4 text-muted-foreground/50" />
            : <IconChevronRight className="h-4 w-4 text-muted-foreground/50" />
        )}
      </button>

      {isExpanded && hasDetails && (
        <div className="border-t border-border/30 bg-background/50 p-3 space-y-2">
          {args && Object.entries(args).map(([key, value]) => {
            const strValue = typeof value === 'string' ? value : JSON.stringify(value, null, 2);
            const isLongValue = strValue.length > 100 || strValue.includes('\n');

            return (
              <div key={key} className="text-xs">
                <span className="font-medium text-muted-foreground">{key}:</span>
                {isLongValue ? (
                  <pre className="mt-1 p-2 bg-muted/50 rounded text-[11px] overflow-x-auto max-h-[200px] overflow-y-auto whitespace-pre-wrap break-all">
                    {strValue}
                  </pre>
                ) : (
                  <span className="ml-2 font-mono text-foreground/80">{strValue}</span>
                )}
              </div>
            );
          })}
          {result && (
            <div className="text-xs border-t border-border/30 pt-2 mt-2">
              <span className="font-medium text-muted-foreground">Result:</span>
              <pre className="mt-1 p-2 bg-muted/50 rounded text-[11px] overflow-x-auto max-h-[150px] overflow-y-auto whitespace-pre-wrap">
                {result}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function TypingIndicator() {
  return (
    <div className="flex items-center gap-2 px-4 py-3 max-w-[85%] rounded-lg bg-muted text-muted-foreground">
      <div className="flex items-center gap-1" role="status" aria-label="Agent is typing">
        <span className="text-[11px] uppercase tracking-wide opacity-70 mr-2">Agent</span>
        <span
          className="w-2 h-2 rounded-full bg-current animate-bounce"
          style={{ animationDelay: '0ms', animationDuration: '1s' }}
        />
        <span
          className="w-2 h-2 rounded-full bg-current animate-bounce"
          style={{ animationDelay: '150ms', animationDuration: '1s' }}
        />
        <span
          className="w-2 h-2 rounded-full bg-current animate-bounce"
          style={{ animationDelay: '300ms', animationDuration: '1s' }}
        />
      </div>
    </div>
  );
}

export function TaskChatPanel({ agents, onSend, isLoading, isAgentWorking, taskDescription }: TaskChatPanelProps) {
  const [messageInput, setMessageInput] = useState('');
  const [selectedAgent, setSelectedAgent] = useState(agents[0]?.id ?? '');
  const [planModeEnabled, setPlanModeEnabled] = useState(false);
  const [isSending, setIsSending] = useState(false);
  const [isAwaitingResponse, setIsAwaitingResponse] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const lastAgentMessageCountRef = useRef(0);

  const commentsState = useAppStore((state) => state.comments);
  const comments = commentsState?.items ?? [];
  const commentsLoading = commentsState?.isLoading ?? false;

  // Filter to only show message, content, and tool_call types (not progress, etc)
  const visibleComments = comments.filter(
    (c) => c.type === 'message' || c.type === 'content' || c.type === 'tool_call' || !c.type
  );

  // Create a synthetic "user" message for the task description
  const taskDescriptionMessage: Comment | null = taskDescription ? {
    id: 'task-description',
    task_id: commentsState?.taskId ?? '',
    author_type: 'user',
    content: taskDescription,
    type: 'message',
    created_at: '',
  } : null;

  // Combine task description with visible comments
  const allMessages = taskDescriptionMessage
    ? [taskDescriptionMessage, ...visibleComments]
    : visibleComments;

  // Count agent messages to detect new responses
  const agentMessageCount = visibleComments.filter((c) => c.author_type !== 'user').length;

  // Clear awaiting state when a new agent message arrives
  useEffect(() => {
    if (agentMessageCount > lastAgentMessageCountRef.current) {
      setIsAwaitingResponse(false);
    }
    lastAgentMessageCountRef.current = agentMessageCount;
  }, [agentMessageCount]);

  // Show typing indicator when awaiting response AND agent session is active
  const showTypingIndicator = isAwaitingResponse && isAgentWorking;

  // Scroll to bottom when new comments arrive or when typing indicator appears
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [visibleComments.length, showTypingIndicator]);

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault();
    const trimmed = messageInput.trim();
    if (!trimmed || isSending) return;
    setIsSending(true);
    setIsAwaitingResponse(true);
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
        {!isLoading && !commentsLoading && allMessages.length === 0 && (
          <div className="flex items-center justify-center py-8 text-muted-foreground">
            <span>No messages yet. Start the conversation!</span>
          </div>
        )}
        {allMessages.map((comment) => {
          // Tool call comments have a special rendering
          if (comment.type === 'tool_call') {
            return <ToolCallCard key={comment.id} comment={comment} />;
          }

          // Regular message or agent response - render as markdown
          const isUser = comment.author_type === 'user';
          const isTaskDescription = comment.id === 'task-description';

          // Determine label and styling
          let label: string;
          let containerClass: string;

          if (isTaskDescription) {
            label = 'Task';
            containerClass = 'ml-auto bg-primary/20 text-foreground border border-primary/40';
          } else if (isUser) {
            label = 'You';
            containerClass = 'ml-auto bg-primary text-primary-foreground';
          } else {
            label = 'Agent';
            containerClass = 'bg-muted text-foreground';
          }

          return (
            <div
              key={comment.id}
              className={cn(
                'max-w-[85%] rounded-lg px-4 py-3 text-sm',
                containerClass
              )}
            >
              <p className={cn(
                "text-[11px] uppercase tracking-wide mb-2",
                isTaskDescription ? "text-primary font-medium" : "opacity-70"
              )}>
                {label}
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
        {showTypingIndicator && <TypingIndicator />}
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

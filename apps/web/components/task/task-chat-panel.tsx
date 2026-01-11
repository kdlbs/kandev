'use client';

import { FormEvent, useState } from 'react';
import { IconBrain, IconListCheck, IconPaperclip } from '@tabler/icons-react';
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

type ChatMessage = {
  id: string;
  role: 'user' | 'agent';
  content: string;
};

type ChatSession = {
  id: string;
  title: string;
  messages: ChatMessage[];
};

type AgentOption = {
  id: string;
  label: string;
};

type TaskChatPanelProps = {
  activeChat?: ChatSession;
  agents: AgentOption[];
  onSend: (message: string) => void;
};

export function TaskChatPanel({ activeChat, agents, onSend }: TaskChatPanelProps) {
  const [messageInput, setMessageInput] = useState('');
  const [selectedAgent, setSelectedAgent] = useState(agents[0]?.id ?? '');
  const [planModeEnabled, setPlanModeEnabled] = useState(false);

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    const trimmed = messageInput.trim();
    if (!trimmed) return;
    onSend(trimmed);
    setMessageInput('');
  };

  return (
    <>
      <div className="flex-1 min-h-0 overflow-y-auto rounded-lg bg-background p-3 space-y-3">
        {(activeChat?.messages ?? []).map((message) => (
          <div
            key={message.id}
            className={cn(
              'max-w-[80%] rounded-lg px-3 py-2 text-sm leading-relaxed',
              message.role === 'user'
                ? 'ml-auto bg-primary text-primary-foreground'
                : 'bg-muted text-foreground'
            )}
          >
            <p className="text-[11px] uppercase tracking-wide mb-1 opacity-70">
              {message.role === 'user' ? 'You' : 'Agent'}
            </p>
            <p>{message.content}</p>
          </div>
        ))}
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

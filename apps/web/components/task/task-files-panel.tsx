'use client';

import type { CSSProperties } from 'react';
import { memo, useState } from 'react';
import {
  IconArrowBackUp,
  IconChevronRight,
  IconExternalLink,
  IconFile,
  IconFolder,
} from '@tabler/icons-react';
import { Badge } from '@kandev/ui/badge';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@kandev/ui/collapsible';
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarProvider,
  SidebarRail,
} from '@kandev/ui/sidebar';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@kandev/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { LineStat } from '@/components/diff-stat';
import { cn } from '@/lib/utils';
import { CHANGED_FILES, FILE_TREE } from '@/components/task/task-data';

type TaskFilesPanelProps = {
  onSelectDiffPath: (path: string) => void;
};

type TreeItem = string | TreeItem[];

const badgeClass = (status: string) =>
  cn(
    'text-[10px] font-semibold',
    status === 'M' && 'bg-yellow-500/15 text-yellow-700',
    status === 'A' && 'bg-emerald-500/15 text-emerald-700',
    status === 'D' && 'bg-rose-500/15 text-rose-700'
  );

const splitPath = (path: string) => {
  const lastSlash = path.lastIndexOf('/');
  if (lastSlash === -1) return { folder: '', file: path };
  return {
    folder: path.slice(0, lastSlash),
    file: path.slice(lastSlash + 1),
  };
};

function Tree({ item }: { item: TreeItem }) {
  const [name, ...items] = Array.isArray(item) ? item : [item];

  if (!items.length) {
    return (
      <SidebarMenuButton className="data-[active=true]:bg-transparent">
        <IconFile className="h-4 w-4" />
        {name}
      </SidebarMenuButton>
    );
  }

  return (
    <SidebarMenuItem>
      <Collapsible
        className="group/collapsible [&[data-state=open]>button>svg:first-child]:rotate-90"
        defaultOpen={name === 'components' || name === 'ui'}
      >
        <CollapsibleTrigger asChild>
          <SidebarMenuButton>
            <IconChevronRight className="h-4 w-4 transition-transform" />
            <IconFolder className="h-4 w-4" />
            {name}
          </SidebarMenuButton>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <SidebarMenuSub>
            {items.map((subItem, index) => (
              <Tree key={index} item={subItem} />
            ))}
          </SidebarMenuSub>
        </CollapsibleContent>
      </Collapsible>
    </SidebarMenuItem>
  );
}

const TaskFilesPanel = memo(function TaskFilesPanel({ onSelectDiffPath }: TaskFilesPanelProps) {
  const [topTab, setTopTab] = useState<'diff' | 'files'>('diff');

  return (
    <div className="h-full min-h-0 bg-card p-4 flex flex-col rounded-lg border border-border/70 border-l-0">
      <Tabs value={topTab} onValueChange={(value) => setTopTab(value as 'diff' | 'files')} className="flex-1 min-h-0">
        <TabsList>
          <TabsTrigger value="diff" className="cursor-pointer">
            Diff files
          </TabsTrigger>
          <TabsTrigger value="files" className="cursor-pointer">
            All files
          </TabsTrigger>
        </TabsList>
        <TabsContent value="diff" className="mt-3 flex-1 min-h-0">
          <div className="flex-1 min-h-0 overflow-y-auto rounded-lg bg-background p-3 h-full">
            <ul className="space-y-2">
              {CHANGED_FILES.map((file) => {
                const { folder, file: name } = splitPath(file.path);
                return (
                  <li
                    key={file.path}
                    className="group flex items-center justify-between gap-3 text-sm rounded-md px-1 py-0.5 -mx-1 hover:bg-muted/60 cursor-pointer"
                    onClick={() => onSelectDiffPath(file.path)}
                  >
                    <button type="button" className="min-w-0 text-left cursor-pointer">
                      <p className="truncate text-foreground">
                        <span className="text-foreground/60">{folder}/</span>
                        <span className="font-medium text-foreground">{name}</span>
                      </p>
                    </button>
                    <div className="flex items-center gap-2">
                      <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100">
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className="text-muted-foreground hover:text-foreground"
                              onClick={(event) => {
                                event.stopPropagation();
                              }}
                            >
                              <IconArrowBackUp className="h-3.5 w-3.5" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent>Discard changes</TooltipContent>
                        </Tooltip>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className="text-muted-foreground hover:text-foreground"
                              onClick={(event) => {
                                event.stopPropagation();
                              }}
                            >
                              <IconExternalLink className="h-3.5 w-3.5" />
                            </button>
                          </TooltipTrigger>
                          <TooltipContent>Open in editor</TooltipContent>
                        </Tooltip>
                      </div>
                      <LineStat added={file.plus} removed={file.minus} />
                      <Badge className={badgeClass(file.status)}>{file.status}</Badge>
                    </div>
                  </li>
                );
              })}
            </ul>
          </div>
        </TabsContent>
        <TabsContent value="files" className="mt-3 flex-1 min-h-0">
          <div className="flex-1 min-h-0 overflow-y-auto rounded-lg bg-background h-full">
            <SidebarProvider
              className="h-full w-full"
              style={{ "--sidebar-width": "100%" } as CSSProperties}
            >
              <Sidebar collapsible="none" className="h-full w-full">
                <SidebarContent>
                  <SidebarGroup>
                    <SidebarGroupContent>
                      <SidebarMenu>
                        {FILE_TREE.map((item, index) => (
                          <Tree key={index} item={item} />
                        ))}
                      </SidebarMenu>
                    </SidebarGroupContent>
                  </SidebarGroup>
                </SidebarContent>
                <SidebarRail />
              </Sidebar>
            </SidebarProvider>
          </div>
        </TabsContent>
      </Tabs>
    </div>
  );
});

export { TaskFilesPanel };

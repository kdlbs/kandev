'use client';

import type { ReactNode, MouseEvent } from 'react';
import { Fragment } from 'react';
import { Tabs, TabsList, TabsTrigger } from '@kandev/ui/tabs';

export type SessionTab = {
  id: string;
  label: string;
  icon?: ReactNode;
  closable?: boolean;
  alwaysShowClose?: boolean;
  onClose?: (event: MouseEvent) => void;
  className?: string;
};

type SessionTabsProps = {
  children?: ReactNode; // TabsContent elements (optional for cases where tabs are just visible)
  tabs: SessionTab[];
  activeTab: string;
  onTabChange: (tabId: string) => void;
  showAddButton?: boolean;
  onAddTab?: () => void;
  addButtonLabel?: string;
  separatorAfterIndex?: number;
  className?: string;
  // Collapse support
  collapsible?: boolean;
  isCollapsed?: boolean;
  onToggleCollapse?: () => void;
  // Right content (e.g., approve button)
  rightContent?: ReactNode;
};

const SVG_PROPS = { xmlns: 'http://www.w3.org/2000/svg', viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: '2', strokeLinecap: 'round' as const, strokeLinejoin: 'round' as const };

function CloseIcon() {
  return <svg className="h-3 w-3" {...SVG_PROPS}><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg>;
}

function CollapseIcon({ isCollapsed }: { isCollapsed: boolean }) {
  return (
    <svg className="h-4 w-4" {...SVG_PROPS}>
      {isCollapsed ? <polyline points="18 15 12 9 6 15" /> : <polyline points="6 9 12 15 18 9" />}
    </svg>
  );
}

function SessionTabItem({ tab, index, separatorAfterIndex }: { tab: SessionTab; index: number; separatorAfterIndex?: number }) {
  return (
    <Fragment key={tab.id}>
      {separatorAfterIndex !== undefined && index === separatorAfterIndex + 1 && <div className="h-4 w-px bg-border mx-1" />}
      <TabsTrigger value={tab.id} className={tab.className + ' group relative py-1 cursor-pointer rounded-sm max-w-[120px]'}>
        {tab.icon}
        <span className={`truncate ${tab.icon ? 'ml-1.5' : ''}`} style={{ textOverflow: 'clip' }}>{tab.label}</span>
        {tab.closable && tab.onClose && (
          <span role="button" tabIndex={-1} className={`absolute right-1 rounded bg-background hover:bg-muted hover:text-foreground text-muted-foreground transition-opacity ${tab.alwaysShowClose ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'}`} onClick={tab.onClose}>
            <CloseIcon />
          </span>
        )}
      </TabsTrigger>
    </Fragment>
  );
}

function SessionTabsHeader({ tabsList, collapsible, isCollapsed, onToggleCollapse, rightContent, hasChildren }: { tabsList: ReactNode; collapsible: boolean; isCollapsed: boolean; onToggleCollapse?: () => void; rightContent?: ReactNode; hasChildren: boolean }) {
  if (collapsible && onToggleCollapse) {
    return (
      <div className={`flex items-center justify-between gap-2 ${hasChildren ? '' : 'p-2'}`}>
        {tabsList}
        <div className="flex items-center gap-2 shrink-0">
          {rightContent}
          <button type="button" className="text-muted-foreground hover:text-foreground cursor-pointer" onClick={onToggleCollapse}>
            <CollapseIcon isCollapsed={isCollapsed} />
          </button>
        </div>
      </div>
    );
  }
  if (rightContent) {
    return <div className="flex items-center justify-between gap-2">{tabsList}<div className="shrink-0">{rightContent}</div></div>;
  }
  return <>{tabsList}</>;
}

export function SessionTabs({ children, tabs, activeTab, onTabChange, showAddButton = false, onAddTab, addButtonLabel = '+', separatorAfterIndex, className, collapsible = false, isCollapsed = false, onToggleCollapse, rightContent }: SessionTabsProps) {
  const tabsList = (
    <TabsList className="p-0 !h-7 rounded-sm overflow-x-auto overflow-y-hidden min-w-0 shrink [&::-webkit-scrollbar]:hidden [-ms-overflow-style:none] [scrollbar-width:none]">
      {tabs.map((tab, index) => <SessionTabItem key={tab.id} tab={tab} index={index} separatorAfterIndex={separatorAfterIndex} />)}
      {showAddButton && onAddTab && (
        <button type="button" onClick={onAddTab} className="inline-flex items-center justify-center whitespace-nowrap rounded-sm px-2 py-1 h-6 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 cursor-pointer hover:bg-muted">
          {addButtonLabel}
        </button>
      )}
    </TabsList>
  );
  return (
    <Tabs value={activeTab} onValueChange={onTabChange} className={className}>
      <SessionTabsHeader tabsList={tabsList} collapsible={collapsible} isCollapsed={isCollapsed} onToggleCollapse={onToggleCollapse} rightContent={rightContent} hasChildren={Boolean(children)} />
      {children}
    </Tabs>
  );
}

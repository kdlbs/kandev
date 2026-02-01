'use client';

import type { ReactNode, MouseEvent } from 'react';
import { Fragment } from 'react';
import { Tabs, TabsList, TabsTrigger } from '@kandev/ui/tabs';

export type SessionTab = {
  id: string;
  label: string;
  icon?: ReactNode;
  closable?: boolean;
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

export function SessionTabs({
  children,
  tabs,
  activeTab,
  onTabChange,
  showAddButton = false,
  onAddTab,
  addButtonLabel = '+',
  separatorAfterIndex,
  className,
  collapsible = false,
  isCollapsed = false,
  onToggleCollapse,
  rightContent,
}: SessionTabsProps) {
  const tabsList = (
    <TabsList className="p-0 !h-6 rounded-sm">
      {tabs.map((tab, index) => (
        <Fragment key={tab.id}>
          {separatorAfterIndex !== undefined && index === separatorAfterIndex + 1 && (
            <div className="h-4 w-px bg-border mx-1" />
          )}
          <TabsTrigger
            value={tab.id}
            className={tab.className + ' py-1 cursor-pointer rounded-sm'}
          >
            {tab.icon}
            <span className={tab.icon ? 'ml-1.5' : undefined}>{tab.label}</span>
            {tab.closable && tab.onClose && (
              <span
                role="button"
                tabIndex={-1}
                className="ml-0.5 opacity-0 group-hover:opacity-100 transition-opacity hover:text-foreground text-muted-foreground"
                onClick={tab.onClose}
              >
                <svg
                  className="h-3.5 w-3.5"
                  xmlns="http://www.w3.org/2000/svg"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                >
                  <line x1="18" y1="6" x2="6" y2="18" />
                  <line x1="6" y1="6" x2="18" y2="18" />
                </svg>
              </span>
            )}
          </TabsTrigger>
        </Fragment>
      ))}
      {showAddButton && onAddTab && (
        <TabsTrigger value="add" onClick={onAddTab} className="cursor-pointer">
          {addButtonLabel}
        </TabsTrigger>
      )}
    </TabsList>
  );

  const collapseButton = collapsible && onToggleCollapse && (
    <button
      type="button"
      className="text-muted-foreground hover:text-foreground cursor-pointer"
      onClick={onToggleCollapse}
    >
      {isCollapsed ? (
        <svg
          className="h-4 w-4"
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <polyline points="18 15 12 9 6 15" />
        </svg>
      ) : (
        <svg
          className="h-4 w-4"
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <polyline points="6 9 12 15 18 9" />
        </svg>
      )}
    </button>
  );

  return (
    <Tabs value={activeTab} onValueChange={onTabChange} className={className}>
      {collapsible ? (
        <div className={`flex items-center justify-between ${children ? '' : 'p-2'}`}>
          {tabsList}
          <div className="flex items-center gap-2">
            {rightContent}
            {collapseButton}
          </div>
        </div>
      ) : rightContent ? (
        <div className="flex items-center justify-between">
          {tabsList}
          {rightContent}
        </div>
      ) : (
        tabsList
      )}
      {children}
    </Tabs>
  );
}

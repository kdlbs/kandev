"use client";

import { createContext, useCallback, useContext, type ReactNode } from "react";
import type { MessageNavigationActions } from "./messages/message-actions";

export type UserMessageNavigationContextValue = {
  canNavigatePrevious: (messageId: string) => boolean;
  canNavigateNext: (messageId: string) => boolean;
  isBusy: boolean;
  goPrevious: (messageId: string) => Promise<void>;
  goNext: (messageId: string) => Promise<void>;
};

const UserMessageNavigationContext = createContext<UserMessageNavigationContextValue | null>(null);

export function UserMessageNavigationProvider({
  value,
  children,
}: {
  value: UserMessageNavigationContextValue;
  children: ReactNode;
}) {
  return (
    <UserMessageNavigationContext.Provider value={value}>
      {children}
    </UserMessageNavigationContext.Provider>
  );
}

export function useUserMessageNavigationActions(
  messageId: string,
): MessageNavigationActions | undefined {
  const navigation = useContext(UserMessageNavigationContext);
  const onPrevious = useCallback(() => {
    void navigation?.goPrevious(messageId);
  }, [messageId, navigation]);
  const onNext = useCallback(() => {
    void navigation?.goNext(messageId);
  }, [messageId, navigation]);
  if (!navigation) return undefined;
  return {
    canNavigatePrevious: navigation.canNavigatePrevious(messageId),
    canNavigateNext: navigation.canNavigateNext(messageId),
    isBusy: navigation.isBusy,
    onPrevious,
    onNext,
  };
}

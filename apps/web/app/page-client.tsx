'use client';

import { useState } from 'react';

import { KanbanWithPreview } from '@/components/kanban-with-preview';
import { OnboardingDialog } from '@/components/onboarding-dialog';
import { getLocalStorage, setLocalStorage } from '@/lib/local-storage';
import { STORAGE_KEYS } from '@/lib/settings/constants';

export function PageClient() {
  const [showOnboarding, setShowOnboarding] = useState(() => {
    if (typeof window === 'undefined') return false;
    const completed = getLocalStorage(STORAGE_KEYS.ONBOARDING_COMPLETED, false);
    return !completed;
  });
  const [boardKey, setBoardKey] = useState(0);

  const handleOnboardingComplete = () => {
    setLocalStorage(STORAGE_KEYS.ONBOARDING_COMPLETED, true);
    setShowOnboarding(false);
    setBoardKey((prev) => prev + 1);
  };

  return (
    <>
      <OnboardingDialog open={showOnboarding} onComplete={handleOnboardingComplete} />
      <KanbanWithPreview key={boardKey} />
    </>
  );
}

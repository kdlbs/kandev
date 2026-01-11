'use client';

import { useEffect, useState } from 'react';

import { KanbanBoard } from '@/components/kanban-board';
import { OnboardingDialog } from '@/components/onboarding-dialog';
import { getLocalStorage, setLocalStorage } from '@/lib/local-storage';
import { STORAGE_KEYS } from '@/lib/settings/constants';

export function PageClient() {
  const [showOnboarding, setShowOnboarding] = useState(false);

  useEffect(() => {
    const completed = getLocalStorage(STORAGE_KEYS.ONBOARDING_COMPLETED, false);
    if (!completed) {
      setShowOnboarding(true);
    }
  }, []);

  const handleOnboardingComplete = () => {
    setLocalStorage(STORAGE_KEYS.ONBOARDING_COMPLETED, true);
    setShowOnboarding(false);
  };

  return (
    <>
      <OnboardingDialog open={showOnboarding} onComplete={handleOnboardingComplete} />
      <KanbanBoard />
    </>
  );
}

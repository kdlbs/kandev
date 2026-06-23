"use client";

import { useEffect, useState } from "react";

export function useTaskCreateDialogPopoverContainer() {
  const [container, setContainer] = useState<HTMLElement | null>(null);

  useEffect(() => {
    setContainer(document.querySelector<HTMLElement>('[data-testid="create-task-dialog"]'));
  }, []);

  return container;
}

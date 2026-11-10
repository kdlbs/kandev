"use client";

import { useEffect, useRef, useState } from "react";

/** External updates refresh pristine drafts and preserve unsaved edits. */
export function useSettingsDraft<T>(preference: T) {
  const [saved, setSaved] = useState(preference);
  const [draft, setDraft] = useState(preference);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  useEffect(() => {
    setSaved((previous) => {
      if (draftRef.current === previous) setDraft(preference);
      return preference;
    });
  }, [preference]);
  return { saved, setSaved, draft, setDraft, isDirty: draft !== saved };
}

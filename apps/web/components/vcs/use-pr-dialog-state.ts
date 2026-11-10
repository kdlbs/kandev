import { useCallback, useRef, useState } from "react";

export function usePRDialogState() {
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [draft, setDraft] = useState(true);
  const [branchPushed, setBranchPushed] = useState(false);
  const [repo, setRepo] = useState<string | undefined>(undefined);
  const owner = useRef<object>({});
  const captureOwner = useCallback(() => owner.current, []);
  const owns = useCallback((candidate: object) => owner.current === candidate, []);
  const openDialog = useCallback((taskTitle?: string, nextRepo?: string) => {
    // Every explicit opening replaces completion authority, even for equal drafts.
    owner.current = {};
    setTitle(taskTitle || "");
    setBody("");
    setBranchPushed(false);
    // Direct onClick bindings may pass an event instead of a repository name.
    setRepo(typeof nextRepo === "string" ? nextRepo : undefined);
    setOpen(true);
  }, []);
  return {
    open,
    setOpen,
    title,
    setTitle,
    body,
    setBody,
    draft,
    setDraft,
    branchPushed,
    setBranchPushed,
    repo,
    openDialog,
    captureOwner,
    owns,
  };
}

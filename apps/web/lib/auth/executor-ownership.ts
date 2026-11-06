export function executorOwnerChoices({
  userId,
  currentOwner,
  parentOwner,
  manager,
  creating,
  people,
}: {
  userId: string;
  currentOwner: string;
  parentOwner: string;
  manager: boolean;
  creating: boolean;
  people: { id: string; name: string }[];
}): string[] {
  const choices = new Set([""]);
  if (manager) {
    for (const person of people) {
      if (!parentOwner || person.id === parentOwner) choices.add(person.id);
    }
  }
  if (userId && (manager || creating || currentOwner === userId)) {
    if (!parentOwner || userId === parentOwner) choices.add(userId);
  }
  if (currentOwner) choices.add(currentOwner);
  return [...choices];
}

export function executorProfileAvailable(
  profileId: string,
  executors: { profiles?: { id: string }[] }[],
): boolean {
  return (
    Boolean(profileId) &&
    executors.some((executor) => executor.profiles?.some((profile) => profile.id === profileId))
  );
}

export function reconcileExecutorOwnerDraft(
  current: { value: string; baseline: string },
  baseline: string,
) {
  if (current.baseline === baseline) return current;
  return { value: current.value === current.baseline ? baseline : current.value, baseline };
}

export function acceptExecutorOwnerSave(
  current: { value: string; baseline: string },
  submitted: string,
) {
  return { ...current, baseline: submitted };
}

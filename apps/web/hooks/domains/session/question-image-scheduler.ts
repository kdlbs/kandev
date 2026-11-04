type ReadGroup = { active: number; waiting: (() => void)[] };
const groups = new Map<string, ReadGroup>();

// Question and choice hooks share a limit for the same question and owner.
export function scheduleQuestionImage(
  key: string,
  signal: AbortSignal,
  read: () => Promise<Blob>,
): Promise<Blob> {
  if (signal.aborted) return Promise.reject(signal.reason);
  const group = groups.get(key) ?? { active: 0, waiting: [] };
  groups.set(key, group);
  return new Promise((resolve, reject) => {
    const cancel = () => {
      group.waiting = group.waiting.filter((entry) => entry !== start);
      reject(signal.reason);
    };
    const start = () => {
      signal.removeEventListener("abort", cancel);
      group.active++;
      void Promise.resolve()
        .then(() => {
          signal.throwIfAborted();
          return read();
        })
        .then(resolve, reject)
        .finally(() => {
          group.active--;
          group.waiting.shift()?.();
          if (!group.active && !group.waiting.length) groups.delete(key);
        });
    };
    if (group.active < 2) start();
    else {
      group.waiting.push(start);
      signal.addEventListener("abort", cancel, { once: true });
    }
  });
}

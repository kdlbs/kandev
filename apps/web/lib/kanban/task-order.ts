type CreatedTask = {
  createdAt?: string;
};

function createdAtTime(task: CreatedTask): number {
  if (!task.createdAt) return Number.NEGATIVE_INFINITY;
  const time = Date.parse(task.createdAt);
  return Number.isNaN(time) ? Number.NEGATIVE_INFINITY : time;
}

export function compareTasksByCreatedDesc(a: CreatedTask, b: CreatedTask): number {
  const aTime = createdAtTime(a);
  const bTime = createdAtTime(b);
  if (bTime > aTime) return 1;
  if (bTime < aTime) return -1;
  return 0;
}

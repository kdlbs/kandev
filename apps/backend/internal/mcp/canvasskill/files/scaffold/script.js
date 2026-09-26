(() => {
  "use strict";

  const contextTarget = document.querySelector("[data-canvas-context]");
  if (!contextTarget) return;

  fetch("./_kandev/v1/context")
    .then((response) => {
      if (!response.ok) throw new Error("context unavailable");
      return response.json();
    })
    .then((context) => {
      contextTarget.textContent = context.task_id ? `Task ${context.task_id}` : "Workspace canvas";
    })
    .catch(() => {
      contextTarget.textContent = "Canvas context unavailable";
    });
})();

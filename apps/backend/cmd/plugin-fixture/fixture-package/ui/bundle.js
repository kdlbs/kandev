/**
 * Native JS UI bundle for the kandev-plugin-e2e fixture plugin
 * (docs/plans/plugins/PLUGIN-API.md). A self-contained ES module — no
 * imports, no bundled React — that calls `window.registerKandevPlugin(id,
 * plugin)` at evaluation time and, on `initialize(registry, host)`,
 * registers a nav item, a top-level route, a `task-sidebar` slot component,
 * a `main-top-bar` slot component, a `task.created` WS handler, and the
 * `open-demo` keybinding (declared in manifest.yaml's `ui.keybindings`,
 * default `mod+shift+j`) which opens a `host.openModal(...)` demo modal.
 * Uses only host.React/host.jsx.
 *
 * It also registers the plugin-hooks surface this fixture exists to drive
 * end to end: a "Notes" task panel (registerTaskPanel, mobile-enabled) that
 * round-trips a single per-user document through host.storage
 * (get on mount, debounced set, subscribe to pick up a write from another
 * tab/surface), a task-card-indicators slot component, and a
 * registerTaskMenuAction under the kanban card's "edit" group.
 *
 * The task-created counter lives in module scope (not component state) with
 * a tiny listener set, so it survives across route navigations (the page
 * component unmounts/remounts as the user navigates away and back).
 */
(function () {
  var moduleCount = 0;
  var listeners = new Set();

  function emit() {
    listeners.forEach(function (fn) {
      fn(moduleCount);
    });
  }

  function incrementCount() {
    moduleCount += 1;
    emit();
  }

  function useCounter(React) {
    var state = React.useState(moduleCount);
    var count = state[0];
    var setCount = state[1];
    React.useEffect(function () {
      setCount(moduleCount);
      listeners.add(setCount);
      return function () {
        listeners.delete(setCount);
      };
    }, []);
    return count;
  }

  window.registerKandevPlugin("kandev-plugin-e2e", {
    initialize: function (registry, host) {
      var React = host.React;
      var jsx = host.jsx;

      function PluginPage() {
        var count = useCounter(React);
        return jsx(
          "div",
          { id: "hello-plugin-page-root" },
          jsx("h1", { id: "hello-plugin-page" }, "Hello E2E"),
          jsx("span", { id: "hello-task-counter" }, String(count)),
        );
      }

      function SidebarSlot() {
        return jsx("div", { id: "hello-sidebar" }, "Hello E2E sidebar");
      }

      function MainTopBarSlot(props) {
        var slotProps = props.slotProps || {};
        return jsx("span", { id: "hello-main-top-bar" }, "Hello " + slotProps.currentPage);
      }

      // Debounce delay for the Notes panel's autosave — short, so e2e specs
      // don't need to wait long for a write to reach host.storage.
      var NOTES_SAVE_DEBOUNCE_MS = 150;

      function useNotesValue(taskId, panelId) {
        var state = React.useState("");
        var value = state[0];
        var setValue = state[1];
        var loadedState = React.useState(false);
        var loaded = loadedState[0];
        var setLoaded = loadedState[1];

        React.useEffect(
          function () {
            var cancelled = false;
            // Each notification (subscribe's refresh callback, plus the initial
            // call) starts its own host.storage.get. Two in flight at once can
            // resolve out of order, so `cancelled` alone (unmount-only) isn't
            // enough — an older read landing after a newer one would overwrite
            // it. Only the read matching the current generation may commit.
            var refreshGeneration = 0;
            function refresh() {
              var generation = ++refreshGeneration;
              host.storage.get("task", taskId, "note").then(
                function (entry) {
                  if (cancelled || generation !== refreshGeneration) return;
                  setValue(entry ? entry.value : "");
                  setLoaded(true);
                },
                function () {
                  if (cancelled || generation !== refreshGeneration) return;
                  setLoaded(true);
                },
              );
            }
            refresh();
            // Scope echo suppression to this panel instance (panelId) rather
            // than the host's shared tab-wide default writerId. The kanban
            // "Enhance notes" action below writes this same note using that
            // shared default (it has no ongoing subscription of its own to
            // protect) — if this panel used the same default, the action's
            // write would look like this panel's own echo and get silently
            // dropped instead of refreshing the panel.
            var unsubscribe = host.storage.subscribe(
              { scope: "task", scopeId: taskId, key: "note", writerId: panelId },
              refresh,
            );
            return function () {
              cancelled = true;
              unsubscribe();
            };
          },
          [taskId, panelId],
        );

        return [value, setValue, loaded];
      }

      function NotesPanel(props) {
        var taskId = props.taskId;
        var panelId = props.panelId;
        var notesValue = useNotesValue(taskId, panelId);
        var value = notesValue[0];
        var setValue = notesValue[1];
        var loaded = notesValue[2];
        var debounceRef = React.useRef(null);

        function handleChange(e) {
          var next = e.target.value;
          setValue(next);
          if (debounceRef.current) clearTimeout(debounceRef.current);
          debounceRef.current = setTimeout(function () {
            host.storage
              .set("task", taskId, "note", next, { writerId: panelId })
              .catch(function () {
                // surface the failed autosave instead of dropping it silently
              });
          }, NOTES_SAVE_DEBOUNCE_MS);
        }

        if (!loaded) {
          return jsx("div", { "data-testid": "e2e-notes-panel-loading" }, "Loading notes…");
        }
        return jsx("textarea", {
          "data-testid": "e2e-notes-panel",
          "data-presentation": props.presentation,
          value: value,
          onChange: handleChange,
        });
      }

      function CardIndicator(props) {
        var slotProps = props.slotProps || {};
        return jsx(
          "span",
          { "data-testid": "e2e-card-indicator", "data-task-id": slotProps.taskId },
          "N",
        );
      }

      function StatusSlot(props) {
        var slotProps = props.slotProps || {};
        var id = slotProps.placement === "left" ? "hello-status-left" : "hello-status-right";
        return jsx(
          "span",
          { id: id },
          "Hello status " +
            String(slotProps.presentation || "unknown") +
            " " +
            String(slotProps.activeTaskId || "no-task"),
        );
      }

      registry.registerNavItem({
        id: "e2e-hello",
        label: "Hello E2E",
        path: "/plugins/e2e-hello",
        section: "main",
      });
      registry.registerRoute("/plugins/e2e-hello", PluginPage);
      registry.registerComponent("task-sidebar", SidebarSlot);
      registry.registerComponent("main-top-bar", MainTopBarSlot);
      registry.registerComponent("app-status-bar-left", StatusSlot);
      registry.registerComponent("app-status-bar-right", StatusSlot);
      registry.registerWsHandler("task.created", function () {
        incrementCount();
      });

      registry.registerTaskPanel({
        id: "notes",
        title: "Notes",
        icon: "book",
        Component: NotesPanel,
        mobileEnabled: true,
      });
      registry.registerComponent("task-card-indicators", CardIndicator);
      registry.registerTaskMenuAction({
        id: "enhance-notes",
        label: "Enhance notes",
        group: "edit",
      run: function (context) {
          return host.storage
            .set("task", context.taskId, "note", "Enhanced via plugin action")
            .then(function () {
              // Keep the received responsive context observable to the E2E
              // fixture without changing the plugin-facing action contract.
              return host.storage.set(
                "task",
                context.taskId,
                "menu-presentation",
                context.presentation,
              );
            });
      },
      });

      registry.registerKeybinding("open-demo", function () {
        function DemoModalContent() {
          return jsx(
            "div",
            { id: "hello-demo-modal", "data-testid": "hello-demo-modal" },
            "Hello from the plugin modal",
          );
        }
        host.openModal({
          title: "Demo Modal",
          content: DemoModalContent,
          size: "md",
        });
      });
    },
  });
})();

/**
 * Native JS UI bundle for the kandev-plugin-e2e fixture plugin
 * (docs/plans/plugins/PLUGIN-API.md). A self-contained ES module — no
 * imports, no bundled React — that calls `window.registerKandevPlugin(id,
 * plugin)` at evaluation time and, on `initialize(registry, host)`,
 * registers a nav item, a top-level route, a `task-sidebar` slot component,
 * a `main-top-bar` slot component, a `task.created` WS handler, and the
 * `open-demo` keybinding (declared in manifest.yaml's `ui.keybindings`,
 * default `mod+shift+k`) which opens a `host.openModal(...)` demo modal.
 * Uses only host.React/host.jsx.
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

      registry.registerNavItem({
        id: "e2e-hello",
        label: "Hello E2E",
        path: "/plugins/e2e-hello",
        section: "main",
      });
      registry.registerRoute("/plugins/e2e-hello", PluginPage);
      registry.registerComponent("task-sidebar", SidebarSlot);
      registry.registerComponent("main-top-bar", MainTopBarSlot);
      registry.registerWsHandler("task.created", function () {
        incrementCount();
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

import { createDemoFiles } from "./demo-files";

function prefixFiles(prefix: string, files: Record<string, string>) {
  return Object.fromEntries(
    Object.entries(files).map(([path, content]) => [`${prefix}/${path}`, content]),
  );
}

export function createDemoMultiRepoFiles(): Record<string, string> {
  return {
    ...prefixFiles("acme-web", createDemoFiles()),
    "acme-api/README.md": `# Acme API

Go services and contracts for the Acme commerce platform.

## Development

Run \`go test ./...\` before opening a pull request. The dashboard contract fixtures are
shared with acme-web and must be regenerated when the client schema changes.
`,
    "acme-api/go.mod": `module github.com/kandev-demo/acme-api

go 1.24

require (
  github.com/go-chi/chi/v5 v5.2.1
  github.com/stretchr/testify v1.10.0
)
`,
    "acme-api/cmd/api/main.go": `package main

import (
  "log"
  "net/http"

  "github.com/kandev-demo/acme-api/internal/router"
)

func main() {
  log.Fatal(http.ListenAndServe(":8080", router.New()))
}
`,
    "acme-api/internal/contracts/dashboard_fixture_test.go": `package contracts

import (
  "testing"

  "github.com/kandev-demo/acme-api/internal/contracts/fixtures"
)

func TestDashboardFixtureMatchesWebClient(t *testing.T) {
  fixtures.AssertMatches(t, "../../../../acme-web/src/api/generated")
}
`,
    "acme-api/internal/contracts/fixtures/dashboard.json": JSON.stringify(
      {
        services: [{ id: "checkout", status: "healthy", latency_ms: 84 }],
        generated_by: "acme-web",
        schema_version: 4,
      },
      null,
      2,
    ),
    "acme-api/internal/http/dashboard_handler.go": `package http

import (
  "encoding/json"
  nethttp "net/http"
)

func Dashboard(w nethttp.ResponseWriter, _ *nethttp.Request) {
  w.Header().Set("Content-Type", "application/json")
  _ = json.NewEncoder(w).Encode(map[string]any{"services": []any{}})
}
`,
    "acme-api/internal/router/router.go": `package router

import (
  "net/http"

  "github.com/go-chi/chi/v5"
  handlers "github.com/kandev-demo/acme-api/internal/http"
)

func New() http.Handler {
  router := chi.NewRouter()
  router.Get("/api/dashboard", handlers.Dashboard)
  return router
}
`,
    "acme-api/openapi/dashboard.yaml": `openapi: 3.1.0
info:
  title: Acme Dashboard API
  version: 1.4.0
paths:
  /api/dashboard:
    get:
      operationId: getDashboard
      responses:
        "200":
          description: Current service health
`,
  };
}

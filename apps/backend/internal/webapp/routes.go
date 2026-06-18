package webapp

import (
	pathpkg "path"
	"strings"
)

// RouteKind describes how the backend should handle an incoming request path.
type RouteKind string

const (
	RouteKindSPA       RouteKind = "spa"
	RouteKindAPI       RouteKind = "api"
	RouteKindWebSocket RouteKind = "websocket"
	RouteKindStatic    RouteKind = "static"
	RouteKindHealth    RouteKind = "health"
)

// RouteName identifies SPA routes that need route-aware boot data.
type RouteName string

const (
	RouteUnknown    RouteName = "unknown"
	RouteHome       RouteName = "home"
	RouteTasks      RouteName = "tasks"
	RouteTaskDetail RouteName = "taskDetail"
	RouteOffice     RouteName = "office"
	RouteSettings   RouteName = "settings"
	RouteGitHub     RouteName = "github"
	RouteGitLab     RouteName = "gitlab"
	RouteJira       RouteName = "jira"
	RouteLinear     RouteName = "linear"
	RouteStats      RouteName = "stats"
)

// RouteClassification is the backend contract for SPA-vs-backend routing.
type RouteClassification struct {
	Kind   RouteKind         `json:"kind"`
	Route  RouteName         `json:"route,omitempty"`
	Path   string            `json:"path"`
	Params map[string]string `json:"params,omitempty"`
}

// ClassifyRoute returns the future webapp handling class for a request path.
func ClassifyRoute(rawPath string) RouteClassification {
	requestPath := normalizePath(rawPath)
	if kind, ok := classifyNonSPA(requestPath); ok {
		return RouteClassification{Kind: kind, Path: requestPath}
	}

	route, params := classifySPARoute(requestPath)
	return RouteClassification{
		Kind:   RouteKindSPA,
		Route:  route,
		Path:   requestPath,
		Params: params,
	}
}

// IsSPARoute reports whether rawPath should serve the SPA shell.
func IsSPARoute(rawPath string) bool {
	return ClassifyRoute(rawPath).Kind == RouteKindSPA
}

func classifyNonSPA(requestPath string) (RouteKind, bool) {
	switch {
	case requestPath == "/api" || strings.HasPrefix(requestPath, "/api/"):
		return RouteKindAPI, true
	case requestPath == "/ws" || strings.HasPrefix(requestPath, "/ws/"):
		return RouteKindWebSocket, true
	case requestPath == "/health":
		return RouteKindHealth, true
	case isStaticPath(requestPath):
		return RouteKindStatic, true
	default:
		return "", false
	}
}

func classifySPARoute(requestPath string) (RouteName, map[string]string) {
	switch {
	case requestPath == "/":
		return RouteHome, nil
	case requestPath == "/tasks":
		return RouteTasks, nil
	case requestPath == "/github":
		return RouteGitHub, nil
	case requestPath == "/gitlab":
		return RouteGitLab, nil
	case requestPath == "/jira":
		return RouteJira, nil
	case requestPath == "/linear":
		return RouteLinear, nil
	case requestPath == "/stats":
		return RouteStats, nil
	case requestPath == "/office" || strings.HasPrefix(requestPath, "/office/"):
		return RouteOffice, nil
	case requestPath == "/settings" || strings.HasPrefix(requestPath, "/settings/"):
		return RouteSettings, nil
	default:
		return classifyTaskRoute(requestPath)
	}
}

func classifyTaskRoute(requestPath string) (RouteName, map[string]string) {
	taskID, ok := cutSingleSegment(requestPath, "/t/")
	if !ok {
		taskID, ok = cutSingleSegment(requestPath, "/tasks/")
	}
	if !ok {
		return RouteUnknown, nil
	}
	return RouteTaskDetail, map[string]string{"taskId": taskID}
}

func cutSingleSegment(requestPath, prefix string) (string, bool) {
	value, ok := strings.CutPrefix(requestPath, prefix)
	if !ok || value == "" || strings.Contains(value, "/") {
		return "", false
	}
	return value, true
}

func isStaticPath(requestPath string) bool {
	if strings.HasPrefix(requestPath, "/assets/") || strings.HasPrefix(requestPath, "/_next/") {
		return true
	}

	switch requestPath {
	case "/apple-icon.png",
		"/apple-touch-icon.png",
		"/favicon.ico",
		"/icon0.svg",
		"/icon1.png",
		"/icon.svg",
		"/manifest.json",
		"/manifest.webmanifest",
		"/robots.txt",
		"/vite.svg":
		return true
	default:
		return false
	}
}

func normalizePath(rawPath string) string {
	if rawPath == "" {
		return "/"
	}
	cleanInput := stripSuffix(rawPath, "?")
	cleanInput = stripSuffix(cleanInput, "#")
	if !strings.HasPrefix(cleanInput, "/") {
		cleanInput = "/" + cleanInput
	}
	return pathpkg.Clean(cleanInput)
}

func stripSuffix(value, marker string) string {
	if idx := strings.Index(value, marker); idx >= 0 {
		return value[:idx]
	}
	return value
}

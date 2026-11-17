package api

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/httpmw"
)

func proxyCredentialRouter(t *testing.T, srv *Server, auth gin.HandlerFunc, kind string, port int) (*gin.Engine, string) {
	t.Helper()
	router := gin.New()
	router.Use(auth)
	if kind == "editor" {
		srv.procMgr.SetVscodeForTest(process.VscodeStatusRunning, port)
		router.Any("/api/v1/vscode/proxy/*path", srv.handleVscodeProxy)
		return router, "/api/v1/vscode/proxy"
	}
	router.Any("/api/v1/port-proxy/:port/*path", srv.handlePortProxy)
	return router, "/api/v1/port-proxy/" + strconv.Itoa(port)
}

func TestPortProxyAndVSCode_ControlAdmissionAndApplicationRequest(t *testing.T) {
	for _, kind := range []string{"port", "editor"} {
		for _, mode := range []string{"static", "rotating", "invalid", "revoked", "disabled Basic", "disabled Bearer"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				testProxyApplicationRequest(t, kind, mode)
			})
		}
	}
}

func testProxyApplicationRequest(t *testing.T, kind, mode string) {
	t.Helper()
	var appHits atomic.Int32
	header := "Bearer control-token"
	srv := newTestServer(t)
	token := "control-token"
	if strings.HasPrefix(mode, "disabled ") {
		token = ""
		header = strings.TrimPrefix(mode, "disabled ") + " app-auth"
	}
	if mode == "rotating" || mode == "revoked" {
		creds := newCredentialState(token)
		srv.SetCredentialSource(creds)
		if mode == "revoked" {
			if _, _, err := creds.Rotate(token); err != nil {
				t.Fatal(err)
			}
		}
	}
	if mode == "invalid" {
		header = "Bearer invalid-control"
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appHits.Add(1)
		wantAuth := ""
		if token == "" {
			wantAuth = header
		}
		if r.Header.Get("Authorization") != wantAuth {
			t.Error("application authorization differs from intended app credential")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if r.Method != http.MethodPut || r.URL.RequestURI() != "/selection?item=a%2Fb" ||
			string(body) != `{"selected":true}` || r.Header.Get("Cookie") != "app=session" ||
			r.Header.Get(httpmw.InterimSettingsInterlockHeader) != "spa-interlock" {
			t.Error("application request fields changed during forwarding")
		}
		if kind == "port" && r.Host != "preview.example:4242" {
			t.Error("port preview host changed")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	router, prefix := proxyCredentialRouter(t, srv, srv.instanceAuth(token), kind, portOf(t, upstream.URL))
	req := httptest.NewRequest(http.MethodPut, prefix+"/selection?item=a%2Fb", strings.NewReader(`{"selected":true}`))
	req.Host = "preview.example:4242"
	req.Header.Set("Authorization", header)
	req.Header.Set("Cookie", "app=session")
	req.Header.Set(httpmw.InterimSettingsInterlockHeader, "spa-interlock")
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	req = req.WithContext(ctx)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	wantStatus, wantHits := http.StatusNoContent, int32(1)
	if mode == "invalid" || mode == "revoked" {
		wantStatus, wantHits = http.StatusUnauthorized, 0
	}
	if response.Code != wantStatus || appHits.Load() != wantHits {
		t.Fatalf("status = %d, application hits = %d; want %d, %d", response.Code, appHits.Load(), wantStatus, wantHits)
	}
}

func TestPortProxyAndVSCode_UpgradeConsumesControlCredentialAndFencesRotation(t *testing.T) {
	for _, kind := range []string{"port", "editor"} {
		t.Run(kind, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "app=session" {
					t.Error("upgraded application received wrong authentication headers")
				}
				upgrader := websocket.Upgrader{Subprotocols: []string{"app-protocol"}, CheckOrigin: func(*http.Request) bool { return true }}
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer func() { _ = conn.Close() }()
				for {
					mt, data, readErr := conn.ReadMessage()
					if readErr != nil || conn.WriteMessage(mt, data) != nil {
						return
					}
				}
			}))
			t.Cleanup(upstream.Close)
			srv := newTestServer(t)
			creds := newCredentialState("control-token")
			srv.SetCredentialSource(creds)
			router, prefix := proxyCredentialRouter(t, srv, srv.instanceAuth(""), kind, portOf(t, upstream.URL))
			forwarded := httptest.NewServer(router)
			t.Cleanup(forwarded.Close)
			dialer := websocket.Dialer{Subprotocols: []string{"app-protocol"}, HandshakeTimeout: 5 * time.Second}
			conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(forwarded.URL, "http")+prefix+"/ws",
				http.Header{"Authorization": {"Bearer control-token"}, "Cookie": {"app=session"}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = conn.Close() })
			if conn.Subprotocol() != "app-protocol" {
				t.Error("application WebSocket negotiation changed")
			}
			assertEchoRoundTrip(t, conn)
			if _, _, err := creds.Rotate("control-token"); err != nil {
				t.Fatal(err)
			}
			assertConnectionClosedByServer(t, conn, 2*time.Second)
		})
	}
}

func TestPortProxyAndVSCode_StreamConsumesControlCredentialAndFencesRotation(t *testing.T) {
	for _, kind := range []string{"port", "editor"} {
		t.Run(kind, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" {
					t.Error("streaming application received control authorization")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintln(w, "data: ready")
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			t.Cleanup(upstream.Close)
			srv := newTestServer(t)
			creds := newCredentialState("control-token")
			srv.SetCredentialSource(creds)
			router, prefix := proxyCredentialRouter(t, srv, srv.instanceAuth(""), kind, portOf(t, upstream.URL))
			forwarded := httptest.NewServer(router)
			t.Cleanup(forwarded.Close)
			req, err := http.NewRequest(http.MethodGet, forwarded.URL+prefix+"/stream", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer control-token")
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = resp.Body.Close() })
			reader := bufio.NewReader(resp.Body)
			if line, err := reader.ReadString('\n'); err != nil || line != "data: ready\n" {
				t.Fatalf("stream first event = %q, error = %v", line, err)
			}
			if _, _, err := creds.Rotate("control-token"); err != nil {
				t.Fatal(err)
			}
			if _, err := reader.ReadString('\n'); err == nil || errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("stream failed to close before client timeout: %v", err)
			}
		})
	}
}

package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestControlInstanceErrorsExposeStatus(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"invalid auth token"}`))
			}))
			t.Cleanup(server.Close)
			client := newTestControlClient(t, server)
			for _, operation := range []func() error{
				func() error { _, err := client.GetInstance(context.Background(), "instance"); return err },
				func() error { return client.DeleteInstance(context.Background(), "instance") },
				func() error {
					_, err := client.CreateInstance(context.Background(), &CreateInstanceRequest{ID: "instance"})
					return err
				},
			} {
				err := operation()
				var response interface{ HTTPStatusCode() int }
				if !errors.As(err, &response) {
					t.Errorf("HTTP status unavailable: %v", err)
					continue
				}
				if response.HTTPStatusCode() != status {
					t.Errorf("got %d, want %d", response.HTTPStatusCode(), status)
				}
			}
		})
	}
}

func TestControlInstanceErrorsExposeStatusWithoutJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, "Unauthorized")
	}))
	t.Cleanup(server.Close)
	client := newTestControlClient(t, server)
	for _, operation := range []func() error{
		func() error { return client.DeleteInstance(context.Background(), "instance") },
		func() error {
			_, err := client.CreateInstance(context.Background(), &CreateInstanceRequest{ID: "instance"})
			return err
		},
	} {
		var response interface{ HTTPStatusCode() int }
		err := operation()
		if !errors.As(err, &response) {
			t.Errorf("HTTP status unavailable: %v", err)
		}
	}
}

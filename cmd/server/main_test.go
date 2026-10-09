package main

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func noEnv(string) string { return "" }

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return addr
}

func waitForServer(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server did not start listening on %s", addr)
}

func TestNewStore(t *testing.T) {
	tests := []struct {
		name        string
		storageType string
		dsn         string
		wantErr     string
	}{
		{name: "memory ok", storageType: "memory"},
		{name: "postgres without dsn", storageType: "postgres", wantErr: "DATABASE_URL is required"},
		{name: "unknown type", storageType: "redis", wantErr: "unknown storage type: redis"},
		{name: "empty type", storageType: "", wantErr: "unknown storage type"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, err := newStore(tc.storageType, tc.dsn)

			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if st == nil {
					t.Fatal("expected non-nil store")
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestRun_InvalidFlag(t *testing.T) {
	err := run(context.Background(), []string{"-no-such-flag"}, noEnv)
	if err == nil {
		t.Fatal("expected error for invalid flag")
	}
}

func TestRun_UnknownStorage(t *testing.T) {
	err := run(context.Background(), []string{"-storage=mongo"}, noEnv)
	if err == nil || !strings.Contains(err.Error(), "unknown storage type") {
		t.Fatalf("got %v, want unknown storage type error", err)
	}
}

func TestRun_PostgresWithoutDatabaseURL(t *testing.T) {
	err := run(context.Background(), []string{"-storage=postgres"}, noEnv)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("got %v, want DATABASE_URL error", err)
	}
}

func TestRun_PortAlreadyInUse(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = run(ctx, []string{"-addr=" + l.Addr().String()}, noEnv)
	if err == nil || !strings.Contains(err.Error(), "server failed") {
		t.Fatalf("got %v, want 'server failed' error", err)
	}
}

func TestRun_StartsAndShutsDownGracefully(t *testing.T) {
	addr := freeAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- run(ctx, []string{"-addr=" + addr, "-base=http://" + addr}, noEnv)
	}()

	waitForServer(t, addr)

	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected clean shutdown, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down in time")
	}

	if conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
		_ = conn.Close()
		t.Fatal("server still accepting connections after shutdown")
	}
}

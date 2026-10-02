package cli

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestServeLocalWorkspace(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close(); _ = writer.Close() }()
	cmd := Command()
	cmd.SetOut(writer)
	cmd.SetArgs([]string{"serve", "--root", t.TempDir(), "--state-dir", t.TempDir(), "--listen", "127.0.0.1:0"})
	done := make(chan error, 1)
	go func() { done <- cmd.ExecuteContext(ctx) }()
	address := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(reader).ReadString('\n'); address <- strings.TrimSpace(line) }()
	select {
	case base := <-address:
		client := http.Client{Timeout: 5 * time.Second}
		resp, e := client.Get(base + "/api/session")
		if e != nil {
			t.Fatal(e)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatal(resp.Status)
		}
	case err := <-done:
		t.Fatalf("serve exited: %v", err)
	case <-time.After(15 * time.Second):
		t.Fatal("serve did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not stop")
	}
}

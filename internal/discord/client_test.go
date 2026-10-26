package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSendPrivateDMWithAttachments(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bot test-secret" {
			t.Error("missing bot authentication")
		}
		switch r.URL.Path {
		case "/users/@me/channels":
			var p map[string]string
			_ = json.NewDecoder(r.Body).Decode(&p)
			if p["recipient_id"] != "123456789" {
				t.Errorf("wrong recipient: %v", p)
			}
			fmt.Fprint(w, `{"id":"987654321"}`)
		case "/channels/987654321/messages":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
				return
			}
			defer r.MultipartForm.RemoveAll()
			var p struct {
				Content string `json:"content"`
				Allowed struct {
					Parse []string `json:"parse"`
				} `json:"allowed_mentions"`
			}
			if err := json.Unmarshal([]byte(r.FormValue("payload_json")), &p); err != nil {
				t.Error(err)
			}
			if p.Content != "ready" || p.Allowed.Parse == nil || len(p.Allowed.Parse) != 0 {
				t.Errorf("bad payload: %+v", p)
			}
			for i, want := range []string{"pdf-data", "tex-data"} {
				f, h, err := r.FormFile(fmt.Sprintf("files[%d]", i))
				if err != nil {
					t.Error(err)
					continue
				}
				b, _ := io.ReadAll(f)
				f.Close()
				if string(b) != want || strings.Contains(h.Filename, "/") {
					t.Error("incorrect attachment")
				}
			}
			fmt.Fprint(w, `{"id":"111"}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client, _ := New("test-secret", "123456789")
	client.baseURL = server.URL
	dir := t.TempDir()
	pdf := filepath.Join(dir, "exam.pdf")
	tex := filepath.Join(dir, "exam.tex")
	os.WriteFile(pdf, []byte("pdf-data"), 0o600)
	os.WriteFile(tex, []byte("tex-data"), 0o600)
	if err := client.Send(context.Background(), "ready", pdf, tex); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestDeliveryFailurePreservesFilesAndRedactsResponse(t *testing.T) {
	for _, status := range []int{401, 403, 413, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/users/@me/channels" {
					fmt.Fprint(w, `{"id":"42"}`)
					return
				}
				w.WriteHeader(status)
				fmt.Fprint(w, "test-secret private course material")
			}))
			defer server.Close()
			c, _ := New("test-secret", "123")
			c.baseURL = server.URL
			path := filepath.Join(t.TempDir(), "exam.pdf")
			os.WriteFile(path, []byte("original"), 0o600)
			err := c.Send(context.Background(), "ready", path)
			if err == nil || strings.Contains(err.Error(), "test-secret") || strings.Contains(err.Error(), "private course") {
				t.Fatalf("error=%v", err)
			}
			data, _ := os.ReadFile(path)
			if string(data) != "original" {
				t.Fatal("artifact changed")
			}
		})
	}
}

func TestInvalidAttachmentDoesNotContactDiscord(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer s.Close()
	c, _ := New("token", "123")
	c.baseURL = s.URL
	if err := c.Send(context.Background(), "ready", filepath.Join(t.TempDir(), "missing.pdf")); err == nil {
		t.Fatal("expected error")
	}
	path := filepath.Join(t.TempDir(), "huge.pdf")
	f, _ := os.Create(path)
	f.Truncate(maxUpload + 1)
	f.Close()
	if err := c.Send(context.Background(), "ready", path); err == nil {
		t.Fatal("expected size error")
	}
	if calls != 0 {
		t.Fatal("contacted Discord for invalid file")
	}
}

func TestConfigurationValidation(t *testing.T) {
	for _, input := range [][2]string{{"", "123"}, {"token", ""}, {"token", "../channel"}, {"token", "0"}} {
		if _, err := New(input[0], input[1]); err == nil {
			t.Errorf("accepted %v", input)
		}
	}
}

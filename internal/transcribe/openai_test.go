package transcribe

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestTranscribeSendsMultipartAudio(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "audio-*.wav")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("RIFF fake wav")); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q", got)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if got := r.FormValue("model"); got != "gpt-transcribe" {
			t.Fatalf("model = %q", got)
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if got, _ := io.ReadAll(f); string(got) != "RIFF fake wav" {
			t.Fatalf("file = %q", got)
		}
		_, _ = w.Write([]byte(`{"text":"hello"}`))
	}))
	defer server.Close()
	c := NewOpenAI("test-key")
	c.http = server.Client()
	// exercise the exact request shape against a test endpoint.
	old := endpointForTest(server.URL)
	defer old()
	text, err := c.Transcribe(context.Background(), file.Name())
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello" {
		t.Fatalf("text = %q", text)
	}
}

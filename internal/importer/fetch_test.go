package importer

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestFetchSystem(t *testing.T) {
	body, err := os.ReadFile("../regadom/testdata/fixture.regadom")
	if err != nil {
		t.Fatal(err)
	}
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != SystemPath || r.Method != http.MethodGet {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if status != http.StatusOK {
			http.Error(w, `{"error":{"code":"x"}}`, status)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	src, err := FetchSystem(context.Background(), srv.Client(), srv.URL+"/")
	if err != nil || src.Kind != "regadom" || len(src.DB.Programs) != 20 {
		t.Fatalf("%+v %v", src, err)
	}
	status = http.StatusNotFound
	if _, err := FetchSystem(context.Background(), srv.Client(), srv.URL); !errors.Is(err, ErrNoSystemDatabase) {
		t.Errorf("none kept: %v", err)
	}
	status = http.StatusForbidden
	if _, err := FetchSystem(context.Background(), srv.Client(), srv.URL); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("refused: %v", err)
	}
	srv.Close()
	if _, err := FetchSystem(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Error("an unreachable system")
	}
	if _, err := FetchSystem(context.Background(), srv.Client(), "http://bad host\x7f"); err == nil {
		t.Error("a URL that is none")
	}
}

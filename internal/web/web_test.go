package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler_ServesIndexAtRoot(t *testing.T) {
	w := get(t, "/")
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content-type: got %q, want text/html*", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<title>homecast</title>") {
		t.Errorf("body missing expected <title>; got first 200 bytes: %q", head(body, 200))
	}
	if !strings.Contains(body, "app.js") {
		t.Errorf("body missing app.js reference; first 200 bytes: %q", head(body, 200))
	}
}

func TestHandler_ServesStyleCSS(t *testing.T) {
	w := get(t, "/style.css")
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body=%q)", w.Code, head(w.Body.String(), 200))
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/css") {
		t.Errorf("content-type: got %q, want text/css*", ct)
	}
	if w.Body.Len() == 0 {
		t.Error("empty style.css body")
	}
}

func TestHandler_ServesAppJS(t *testing.T) {
	w := get(t, "/app.js")
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body=%q)", w.Code, head(w.Body.String(), 200))
	}
	ct := w.Header().Get("Content-Type")
	// Go's mime package reports text/javascript for .js.
	if !strings.Contains(ct, "javascript") {
		t.Errorf("content-type: got %q, want *javascript*", ct)
	}
	if !strings.Contains(w.Body.String(), "/api/") {
		t.Errorf("app.js should call the API; got first 200 bytes: %q", head(w.Body.String(), 200))
	}
}

func TestHandler_404OnMissing(t *testing.T) {
	w := get(t, "/does-not-exist.txt")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", w.Code)
	}
}

// TestHandler_NoDirectoryListing guards against http.FileServer's default
// behaviour of rendering a clickable HTML index for directories. We have no
// subdirectories today, but the regression would show up the moment we add
// one. Requesting "/" is still fine — it resolves to index.html.
func TestHandler_NoDirectoryListing(t *testing.T) {
	// Simulate a directory request by appending a trailing slash to a name
	// that does not exist. http.FileServer should 404, not render a listing.
	w := get(t, "/subdir/")
	if w.Code == http.StatusOK && strings.Contains(w.Body.String(), "<pre>") {
		t.Fatalf("directory listing rendered for /subdir/: %q", head(w.Body.String(), 200))
	}
}

func TestFS_ExposesStaticAssets(t *testing.T) {
	// Callers (cmd/homecast in slice 4) may want to walk or sub the FS; make
	// sure the exported FS lets them see index.html at the root.
	f, err := FS.Open("index.html")
	if err != nil {
		t.Fatalf("open index.html via FS: %v", err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		t.Fatalf("stat index.html: %v", err)
	}
	if info.IsDir() {
		t.Fatal("index.html is a directory?")
	}
	if info.Size() == 0 {
		t.Fatal("index.html is empty")
	}

	// Confirm FS is a proper fs.FS implementation (fs.ReadFile works).
	if _, err := fs.ReadFile(FS, "style.css"); err != nil {
		t.Errorf("fs.ReadFile(style.css): %v", err)
	}
}

// --- helpers -------------------------------------------------------------

func get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, r)
	return w
}

func head(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

package graph

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/upex-galaxy/jitso/internal/orca"
	"github.com/upex-galaxy/jitso/internal/registry"
)

const (
	port  = 4711
	token = "0123456789abcdef"
)

// view is a snapshot with one listed terminal and one gone launch.
func view() Snapshot {
	return Snapshot{Orca: true, Notices: []string{}, Hierarchy: []Hierarchy{}, Synapses: []Synapse{}, Nodes: []Node{
		{Key: "term_a", Handle: "term_a", State: "unlaunched", Listed: true, Name: `<img src=x onerror=alert(1)>`},
		{Key: "L1", ID: "L1", Handle: "term_gone", State: "gone"},
	}}
}

type switches struct {
	handles []string
	err     error
}

func (s *switches) to(handle string) error {
	s.handles = append(s.handles, handle)
	return s.err
}

func handlerFor(sw *switches) http.Handler {
	return Server{Port: port, Token: token, Snapshot: func() (Snapshot, error) { return view(), nil }, Switch: sw.to}.Handler()
}

type call struct {
	method, path, host, body string
	header                   map[string]string
}

func serve(h http.Handler, c call) *httptest.ResponseRecorder {
	r := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
	r.Host = c.host
	if c.host == "" {
		r.Host = "127.0.0.1:" + strconv.Itoa(port)
	}
	for k, v := range c.header {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestHostCheck(t *testing.T) {
	sw := &switches{}
	h := handlerFor(sw)
	tests := []struct {
		host string
		want int
	}{
		{"127.0.0.1:4711", 200},
		{"localhost:4711", 200},
		{"LOCALHOST:4711", 200},
		{"127.0.0.1:4712", 403},
		{"127.0.0.1", 403},
		{"localhost", 403},
		{"evil.example:4711", 403},
		{"127.0.0.1.evil.example:4711", 403},
		{"localhost.evil.example:4711", 403},
		{"[::1]:4711", 403},
		{"0.0.0.0:4711", 403},
	}
	for _, tt := range tests {
		for _, path := range []string{"/", "/app.js", "/style.css", "/api/snapshot"} {
			w := serve(h, call{method: "GET", path: path, host: tt.host})
			if w.Code != tt.want {
				t.Errorf("%s with host %q: %d, want %d", path, tt.host, w.Code, tt.want)
			}
			if tt.want == 403 && (strings.Contains(w.Body.String(), token) || strings.Contains(w.Body.String(), "term_a")) {
				t.Errorf("%s with host %q leaked: %s", path, tt.host, w.Body)
			}
		}
		w := serve(h, call{method: "POST", path: "/api/focus", host: tt.host, body: `{"handle":"term_a"}`, header: map[string]string{TokenHeader: token}})
		if w.Code != tt.want {
			t.Errorf("focus with host %q: %d, want %d", tt.host, w.Code, tt.want)
		}
	}
	// Only the three accepted hosts switched.
	if len(sw.handles) != 3 {
		t.Errorf("switched %v", sw.handles)
	}
}

func TestDefaultPortMayBeLeftOut(t *testing.T) {
	h := Server{Port: 80, Token: token, Snapshot: func() (Snapshot, error) { return view(), nil }}.Handler()
	if w := serve(h, call{method: "GET", path: "/", host: "localhost"}); w.Code != 200 {
		t.Errorf("port 80 without a port: %d", w.Code)
	}
}

func TestPageHoldsTheTokenAndNoResponseAllowsAnotherOrigin(t *testing.T) {
	h := handlerFor(&switches{})
	page := serve(h, call{method: "GET", path: "/"})
	if !strings.Contains(page.Body.String(), `content="`+token+`"`) || strings.Contains(page.Body.String(), tokenMark) {
		t.Error("the page must receive the token")
	}
	for _, c := range []call{
		{method: "GET", path: "/"},
		{method: "GET", path: "/app.js"},
		{method: "GET", path: "/style.css"},
		{method: "GET", path: "/api/snapshot", header: map[string]string{"Origin": "http://evil.example"}},
		{method: "OPTIONS", path: "/api/focus", header: map[string]string{"Origin": "http://evil.example", "Access-Control-Request-Method": "POST"}},
		{method: "GET", path: "/nothing"},
	} {
		w := serve(h, c)
		for name := range w.Header() {
			if strings.HasPrefix(strings.ToLower(name), "access-control-") {
				t.Errorf("%s %s sends %s", c.method, c.path, name)
			}
		}
		if w.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "default-src 'none'") {
			t.Errorf("%s %s: security headers %v", c.method, c.path, w.Header())
		}
		if c.path != "/" && strings.Contains(w.Body.String(), token) {
			t.Errorf("%s %s holds the token", c.method, c.path)
		}
	}
	if w := serve(h, call{method: "GET", path: "/nothing"}); w.Code != 404 {
		t.Errorf("unknown path: %d", w.Code)
	}
}

func TestFocus(t *testing.T) {
	good := map[string]string{TokenHeader: token}
	tests := []struct {
		name     string
		c        call
		want     int
		switched []string
	}{
		{"listed terminal", call{method: "POST", body: `{"handle":"term_a"}`, header: good}, 200, []string{"term_a"}},
		{"own origin", call{method: "POST", body: `{"handle":"term_a"}`, header: map[string]string{TokenHeader: token, "Origin": "http://localhost:4711"}}, 200, []string{"term_a"}},
		{"another origin", call{method: "POST", body: `{"handle":"term_a"}`, header: map[string]string{TokenHeader: token, "Origin": "http://evil.example"}}, 403, nil},
		{"null origin", call{method: "POST", body: `{"handle":"term_a"}`, header: map[string]string{TokenHeader: token, "Origin": "null"}}, 403, nil},
		{"no token", call{method: "POST", body: `{"handle":"term_a"}`}, 403, nil},
		{"wrong token", call{method: "POST", body: `{"handle":"term_a"}`, header: map[string]string{TokenHeader: "x"}}, 403, nil},
		{"token in the body only", call{method: "POST", body: `{"handle":"term_a","token":"` + token + `"}`}, 403, nil},
		{"get", call{method: "GET", header: good}, 405, nil},
		{"put", call{method: "PUT", body: `{"handle":"term_a"}`, header: good}, 405, nil},
		{"delete", call{method: "DELETE", header: good}, 405, nil},
		{"unknown handle", call{method: "POST", body: `{"handle":"term_z"}`, header: good}, 404, nil},
		{"handle of a session Orca does not list", call{method: "POST", body: `{"handle":"term_gone"}`, header: good}, 404, nil},
		{"empty handle", call{method: "POST", body: `{}`, header: good}, 404, nil},
		{"not JSON", call{method: "POST", body: `term_a`, header: good}, 400, nil},
		{"too large", call{method: "POST", body: `{"handle":"` + strings.Repeat("a", 10000) + `"}`, header: good}, 400, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sw := &switches{}
			tt.c.path = "/api/focus"
			w := serve(handlerFor(sw), tt.c)
			if w.Code != tt.want || !reflect.DeepEqual(sw.handles, tt.switched) {
				t.Errorf("%d %v, want %d %v (%s)", w.Code, sw.handles, tt.want, tt.switched, w.Body)
			}
		})
	}
	t.Run("orca fails", func(t *testing.T) {
		sw := &switches{err: errors.New("orca terminal switch failed: gone")}
		w := serve(handlerFor(sw), call{method: "POST", path: "/api/focus", body: `{"handle":"term_a"}`, header: good})
		if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "orca terminal switch failed") {
			t.Errorf("%d %s", w.Code, w.Body)
		}
	})
	t.Run("an empty token accepts nothing", func(t *testing.T) {
		sw := &switches{}
		h := Server{Port: port, Snapshot: func() (Snapshot, error) { return view(), nil }, Switch: sw.to}.Handler()
		w := serve(h, call{method: "POST", path: "/api/focus", body: `{"handle":"term_a"}`})
		if w.Code != 403 || sw.handles != nil {
			t.Errorf("%d %v", w.Code, sw.handles)
		}
	})
}

// No response of the server holds what a session said, even when Orca puts it
// in the inbox rows or in the output of a failed call.
func TestNoMessageContentInResponses(t *testing.T) {
	rows := `{"handle":"term_a","title":"a"},{"handle":"term_b","title":"b"}`
	message := `{"from_handle":"term_a","to_handle":"term_b","type":"status","created_at":"2026-11-25T10:00:00Z",` +
		`"subject":"LEAK-SUBJECT","body":"LEAK-BODY","payload":"LEAK-PAYLOAD","thread_id":"LEAK-THREAD"}`
	inboxes := map[string]orca.Result{
		"rows":           {Status: 0, Stdout: `{"ok":true,"result":{"messages":[` + message + `]}}`},
		"error":          {Status: 1, Stdout: `{"ok":false,"error":{"code":"bad","message":"LEAK-BODY"},"result":{"messages":[` + message + `]}}`, Stderr: "LEAK-STDERR"},
		"cut off":        {Status: 1, Stdout: `{"ok":true,"result":{"messages":[` + message, Stderr: "LEAK-STDERR"},
		"exit after ok":  {Status: 2, Stdout: `{"ok":true,"result":{"messages":[` + message + `]}}`, Stderr: "LEAK-STDERR"},
		"unexpected row": {Status: 0, Stdout: `{"ok":true,"result":{"messages":[{"from_handle":{"x":"LEAK-BODY"}}]}}`},
	}
	for name, inbox := range inboxes {
		t.Run(name, func(t *testing.T) {
			client := orca.NewClient(func(args []string) orca.Result {
				if args[0] == "terminal" {
					return orca.Result{Stdout: `{"ok":true,"result":{"terminals":[` + rows + `]}}`}
				}
				return inbox
			})
			c := &Collector{
				Store: registry.NewFileStore(t.TempDir()),
				Orca:  func() (Orca, error) { return client, nil },
				Now:   time.Now,
			}
			cache := &Cache{TTL: time.Hour, Now: time.Now, Load: c.Snapshot}
			h := Server{Port: port, Token: token, Snapshot: cache.Get, Switch: func(string) error { return nil }}.Handler()
			w := serve(h, call{method: "GET", path: "/api/snapshot"})
			var got Snapshot
			if w.Code != 200 {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if err := json.Unmarshal(w.Body.Bytes(), &struct {
				Synapses *[]Synapse `json:"synapses"`
			}{&got.Synapses}); err != nil {
				t.Fatal(err)
			}
			if name == "rows" && len(got.Synapses) != 1 {
				t.Errorf("synapses %+v", got.Synapses)
			}
			if strings.Contains(w.Body.String(), "LEAK") {
				t.Errorf("content in the response: %s", w.Body)
			}
			snapshot, err := cache.Get()
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.Failure != nil && strings.Contains(snapshot.Failure.Error(), "LEAK") {
				t.Errorf("content in the error: %v", snapshot.Failure)
			}
		})
	}
}

func TestSnapshotError(t *testing.T) {
	h := Server{Port: port, Token: token, Snapshot: func() (Snapshot, error) { return Snapshot{}, errors.New("cannot read") }}.Handler()
	w := serve(h, call{method: "GET", path: "/api/snapshot"})
	if w.Code != 500 || !strings.Contains(w.Body.String(), "cannot read") {
		t.Errorf("%d %s", w.Code, w.Body)
	}
}

func TestCache(t *testing.T) {
	now := time.Unix(0, 0)
	var loads atomic.Int32
	fail := false
	c := &Cache{TTL: time.Second, Now: func() time.Time { return now }, Load: func() (Snapshot, error) {
		loads.Add(1)
		if fail {
			return Snapshot{}, errors.New("boom")
		}
		return view(), nil
	}}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Get(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if loads.Load() != 1 {
		t.Errorf("%d loads for simultaneous requests, want 1", loads.Load())
	}
	now = now.Add(999 * time.Millisecond)
	c.Get()
	if loads.Load() != 1 {
		t.Error("a fresh snapshot was loaded again")
	}
	now = now.Add(time.Millisecond)
	fail = true
	if _, err := c.Get(); err == nil {
		t.Error("a failed load must be the error")
	}
	fail = false
	if _, err := c.Get(); err != nil || loads.Load() != 3 {
		t.Errorf("a failed load must not be cached: %v, %d loads", err, loads.Load())
	}
}

func TestListenIsLocalOnlyAndServeStops(t *testing.T) {
	ln, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	if addr.IP.String() != "127.0.0.1" || addr.Port == 0 {
		t.Fatalf("listening on %v, want 127.0.0.1 and a port", addr)
	}
	secret, err := NewToken(rand.Reader)
	if err != nil || len(secret) != 64 {
		t.Fatalf("token %q %v", secret, err)
	}
	h := Server{Port: addr.Port, Token: secret, Snapshot: func() (Snapshot, error) { return view(), nil }}.Handler()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, h) }()
	res, err := http.Get("http://" + ln.Addr().String() + "/api/snapshot")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), `"term_a"`) {
		t.Errorf("%d %s", res.StatusCode, body)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not stop")
	}
	if _, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second); err == nil {
		t.Error("the port is still open")
	}
}

func TestNewTokenNeedsRandomness(t *testing.T) {
	if _, err := NewToken(strings.NewReader("short")); err == nil {
		t.Error("a short random source must be an error")
	}
}

// The page is self contained and never builds markup from text.
func TestPageIsSelfContained(t *testing.T) {
	err := fs.WalkDir(web, "web", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := web.ReadFile(path)
		if err != nil {
			return err
		}
		for _, banned := range []string{"http://", "https://", "//cdn", "@import", "url(", "innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval("} {
			if strings.Contains(string(data), banned) {
				t.Errorf("%s holds %q", path, banned)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

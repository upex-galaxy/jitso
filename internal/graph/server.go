package graph

import (
	"bytes"
	"context"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/upex-galaxy/jitso/internal/jsonx"
)

// The page: plain HTML, CSS and JavaScript shipped inside the binary. It loads
// nothing from anywhere but this server.
//
//go:embed web
var web embed.FS

// Host is the one interface the server listens on.
const Host = "127.0.0.1"

// TokenHeader carries the run's token on the one request that acts.
const TokenHeader = "X-Jitso-Token"

// tokenMark is the text of index.html the token replaces.
const tokenMark = "__JITSO_TOKEN__"

// CacheTTL is how long a snapshot is served before Orca is asked again.
const CacheTTL = 2 * time.Second

// Cache serves one snapshot for TTL and loads one at a time: a request that
// arrives during a load waits for it and gets its answer.
type Cache struct {
	TTL  time.Duration
	Now  func() time.Time
	Load func() (Snapshot, error)

	mu     sync.Mutex
	loaded bool
	at     time.Time
	last   Snapshot
}

// Get is the cached snapshot, or a new one when it is older than TTL. A load
// that fails is not cached.
func (c *Cache) Get() (Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loaded && c.Now().Sub(c.at) < c.TTL {
		return c.last, nil
	}
	s, err := c.Load()
	if err != nil {
		return Snapshot{}, err
	}
	// The age counts from the end of the load, so a slow one is not stale at
	// once.
	c.last, c.at, c.loaded = s, c.Now(), true
	return s, nil
}

// NewToken is a random token of 32 bytes in hex.
func NewToken(random io.Reader) (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(random, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Server is what the handler needs.
type Server struct {
	// Port is the port the listener holds; the Host header must name it.
	Port int
	// Token is the run's secret: the page receives it and sends it back to
	// switch terminals.
	Token string
	// Snapshot is the graph now (Cache.Get).
	Snapshot func() (Snapshot, error)
	// Switch shows a terminal in Orca.
	Switch func(handle string) error
}

// Handler serves the page, the snapshot and the switch:
//
//	GET  /              the page, with the token
//	GET  /app.js        its script
//	GET  /style.css     its style
//	GET  /api/snapshot  the snapshot, the JSON of jitso graph --json
//	POST /api/focus     {"handle": "..."}: switch Orca to that terminal
//
// Every request must name this server in its Host header, as 127.0.0.1 or
// localhost with the port, so a page of another site whose name was pointed at
// 127.0.0.1 (DNS rebinding) is refused. A switch also needs the token, comes
// from this origin when the browser names one, and takes only a handle of the
// current snapshot. No response carries a CORS header.
func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		page, err := web.ReadFile("web/index.html")
		if err != nil {
			http.Error(w, "the page is missing", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(bytes.Replace(page, []byte(tokenMark), []byte(s.Token), 1))
	})
	mux.HandleFunc("GET /app.js", asset("web/app.js", "text/javascript; charset=utf-8"))
	mux.HandleFunc("GET /style.css", asset("web/style.css", "text/css; charset=utf-8"))
	mux.HandleFunc("GET /api/snapshot", func(w http.ResponseWriter, r *http.Request) {
		snapshot, err := s.Snapshot()
		if err != nil {
			reply(w, http.StatusInternalServerError, problem(err.Error()))
			return
		}
		reply(w, http.StatusOK, snapshot)
	})
	mux.HandleFunc("POST /api/focus", s.focus)

	hosts := []string{net.JoinHostPort(Host, strconv.Itoa(s.Port)), net.JoinHostPort("localhost", strconv.Itoa(s.Port))}
	if s.Port == 80 {
		// A browser leaves the default port out of the header.
		hosts = append(hosts, Host, "localhost")
	}
	ours := func(host string) bool {
		for _, h := range hosts {
			if strings.EqualFold(host, h) {
				return true
			}
		}
		return false
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		if !ours(r.Host) {
			reply(w, http.StatusForbidden, problem("this server answers only to 127.0.0.1 and localhost on its port"))
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && r.Method != http.MethodGet && r.Method != http.MethodHead {
			if host, ok := strings.CutPrefix(origin, "http://"); !ok || !ours(host) {
				reply(w, http.StatusForbidden, problem("the request comes from another origin"))
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func (s Server) focus(w http.ResponseWriter, r *http.Request) {
	if s.Token == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get(TokenHeader)), []byte(s.Token)) != 1 {
		reply(w, http.StatusForbidden, problem("the token is missing or old: reload the page"))
		return
	}
	var body struct {
		Handle string `json:"handle"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		reply(w, http.StatusBadRequest, problem(`the body must be {"handle": "<terminal handle>"}`))
		return
	}
	snapshot, err := s.Snapshot()
	if err != nil {
		reply(w, http.StatusInternalServerError, problem(err.Error()))
		return
	}
	if !snapshot.Focusable(body.Handle) {
		reply(w, http.StatusNotFound, problem("no session of the current view has that terminal"))
		return
	}
	if err := s.Switch(body.Handle); err != nil {
		reply(w, http.StatusBadGateway, problem(err.Error()))
		return
	}
	reply(w, http.StatusOK, jsonx.Object{{Key: "ok", Value: true}})
}

func asset(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := web.ReadFile(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Write(data)
	}
}

func problem(text string) any { return jsonx.Object{{Key: "error", Value: text}} }

func reply(w http.ResponseWriter, code int, body any) {
	data, err := jsonx.Marshal(body)
	if err != nil {
		code, data = http.StatusInternalServerError, []byte(`{"error":"the answer could not be written"}`)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	w.Write(data)
}

// Listen opens the listener of the server on 127.0.0.1 and no other interface,
// on the given port or, for 0, on a free one.
func Listen(port int) (net.Listener, error) {
	return net.Listen("tcp4", net.JoinHostPort(Host, strconv.Itoa(port)))
}

// Serve serves handler on ln until ctx is done, then closes ln and returns nil.
func Serve(ctx context.Context, ln net.Listener, handler http.Handler) error {
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: time.Minute}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ln) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
	}
	stop, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.Shutdown(stop); err != nil {
		server.Close()
	}
	if err := <-done; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

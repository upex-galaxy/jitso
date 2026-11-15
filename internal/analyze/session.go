package analyze

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/upex-galaxy/jitso/internal/jsonx"
	"github.com/upex-galaxy/jitso/internal/registry"
)

// Analyzer is the analyzer id every analyzed event carries.
const Analyzer = "jitso-analyze@1"

// ModelTotals are the totals of one (thread, family) bucket, in the key order
// the oracle writes.
type ModelTotals struct {
	Requests     int     `json:"requests"`
	Input        float64 `json:"input"`
	CacheRead    float64 `json:"cache_read"`
	CacheWrite5m float64 `json:"cache_write_5m"`
	CacheWrite1h float64 `json:"cache_write_1h"`
	Output       float64 `json:"output"`
	USD          float64 `json:"usd"`
}

// Tokens are the token totals of a session (the two cache writes are summed).
type Tokens struct {
	Input      float64 `json:"input"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
	Output     float64 `json:"output"`
}

// Analysis is the closed set of counts a transcript yields. It never holds a
// line, a path, a command or a tool argument: secret-path calls are only
// counted. Its JSON field order is the oracle's.
type Analysis struct {
	Session string `json:"session"`
	Files   int    `json:"files"`
	// MainRequests are the requests of the main thread.
	MainRequests int `json:"main_requests"`
	// Requests are the main thread plus the subagents.
	Requests             int     `json:"requests"`
	MaxMainContextTokens float64 `json:"max_main_context_tokens"`
	SubagentFiles        int     `json:"subagent_files"`
	// ToolUseCounts maps a tool name to its call count, most used first.
	ToolUseCounts jsonx.Object `json:"tool_use_counts"`
	First         *string      `json:"first"`
	Last          *string      `json:"last"`
	DurationMin   *float64     `json:"duration_min"`
	// ByModel maps "<main|sub>:<family>" to its ModelTotals, in key order.
	ByModel         jsonx.Object `json:"by_model"`
	Tokens          Tokens       `json:"tokens"`
	CostUSDList     float64      `json:"cost_usd_list"`
	SecretPathReads int          `json:"secret_path_reads"`
}

// Session analyzes the main transcript <dir>/<uuid>.jsonl and the subagent
// transcripts in <dir>/<uuid>/subagents/*.jsonl.
func Session(mainPath string, prices Prices) (Analysis, error) {
	if _, err := os.Stat(mainPath); err != nil {
		return Analysis{}, fmt.Errorf("jitso: the transcript %s does not exist", filepath.Base(mainPath))
	}
	session := strings.TrimSuffix(filepath.Base(mainPath), ".jsonl")
	subDir := filepath.Join(filepath.Dir(mainPath), session, "subagents")
	var subs []string
	if _, err := os.Stat(subDir); err == nil {
		entries, err := os.ReadDir(subDir)
		if err != nil {
			return Analysis{}, err
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".jsonl") {
				subs = append(subs, filepath.Join(subDir, e.Name()))
			}
		}
	}
	files := append([]string{mainPath}, subs...)

	type bucket struct {
		input, cacheRead, write5m, write1h, output float64
		requests                                   int
	}
	tools := newToolCounts()
	var stamps []string
	totals := map[string]*bucket{}
	var peak float64
	var mainRequests, requests, secretReads int
	for i, path := range files {
		kind := "sub"
		if i == 0 {
			kind = "main"
		}
		scanned, err := scanFile(path, tools, &stamps)
		if err != nil {
			return Analysis{}, err
		}
		secretReads += scanned.secretReads
		for _, r := range scanned.rows {
			key := kind + ":" + r.family
			b := totals[key]
			if b == nil {
				b = &bucket{}
				totals[key] = b
			}
			b.input += r.input
			b.cacheRead += r.cacheRead
			b.write5m += r.write5m
			b.write1h += r.write1h
			b.output += r.output
			b.requests++
			requests++
			if kind == "main" {
				mainRequests++
				peak = max(peak, r.input+r.cacheRead+r.write5m+r.write1h)
			}
		}
	}

	keys := make([]string, 0, len(totals))
	for k := range totals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var cost float64
	var tokens Tokens
	byModel := make(jsonx.Object, 0, len(keys))
	for _, key := range keys {
		a := totals[key]
		p, err := prices.For(key[strings.Index(key, ":")+1:])
		if err != nil {
			return Analysis{}, err
		}
		usd := (a.input*p.Input + a.cacheRead*p.CacheRead + a.write5m*p.CacheWrite5m + a.write1h*p.CacheWrite1h + a.output*p.Output) / 1e6
		cost += usd
		tokens.Input += a.input
		tokens.CacheRead += a.cacheRead
		tokens.CacheWrite += a.write5m + a.write1h
		tokens.Output += a.output
		byModel = append(byModel, jsonx.Field{Key: key, Value: ModelTotals{
			Requests: a.requests, Input: a.input, CacheRead: a.cacheRead, CacheWrite5m: a.write5m,
			CacheWrite1h: a.write1h, Output: a.output, USD: round3(usd),
		}})
	}

	out := Analysis{
		Session:              session,
		Files:                len(files),
		MainRequests:         mainRequests,
		Requests:             requests,
		MaxMainContextTokens: peak,
		SubagentFiles:        len(subs),
		ToolUseCounts:        toolObject(tools),
		ByModel:              byModel,
		Tokens:               tokens,
		CostUSDList:          round3(cost),
		SecretPathReads:      secretReads,
	}
	sort.Strings(stamps)
	if len(stamps) > 0 {
		first, last := stamps[0], stamps[len(stamps)-1]
		out.First, out.Last = &first, &last
		out.DurationMin = durationMin(first, last)
	}
	return out, nil
}

// scanFile opens one transcript and scans it as a stream.
func scanFile(path string, tools *toolCounts, stamps *[]string) (scanResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return scanResult{}, err
	}
	defer f.Close()
	return scan(f, tools, stamps)
}

// toolObject is the tool counts, most used first; equal counts keep the order
// they were first seen in (a stable sort, as JavaScript's).
func toolObject(t *toolCounts) jsonx.Object {
	names := append([]string(nil), t.order...)
	sort.SliceStable(names, func(i, j int) bool { return t.counts[names[i]] > t.counts[names[j]] })
	obj := make(jsonx.Object, 0, len(names))
	for _, n := range names {
		obj = append(obj, jsonx.Field{Key: n, Value: t.counts[n]})
	}
	return obj
}

// durationMin is the minutes between two timestamps to one decimal, nil when
// either does not parse or the span is not positive.
func durationMin(first, last string) *float64 {
	a, okA := registry.ParseTime(first)
	b, okB := registry.ParseTime(last)
	if !okA || !okB {
		return nil
	}
	seconds := float64(b.UnixMilli()-a.UnixMilli()) / 1000
	if !(seconds > 0) {
		return nil
	}
	m := jsRound(seconds/60*10) / 10
	return &m
}

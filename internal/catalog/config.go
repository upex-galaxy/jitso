package catalog

import (
	"math"
	"strconv"
	"strings"

	"github.com/upex-galaxy/jitso/internal/jsconv"
)

// Config is jitso/jitso.yaml: the allowlists every build and every request
// override is checked against.
type Config struct {
	// Version is Number(version): NaN when it is missing or not a number.
	Version float64
	Models  []string
	Efforts []string
	MCP     []string
	Skills  []string
	Tools   []string
}

// configFrom reads the allowlists from a decoded jitso.yaml the way the
// oracle's loadConfig does: each list through String(), anything that is not
// a list is an empty list. A top level that is not a mapping is an error (the
// oracle crashes on it).
func configFrom(raw any) (Config, bool) {
	m, ok := raw.(*Map)
	if !ok {
		return Config{}, false
	}
	return Config{
		Version: jsNumber(m.Value("version")),
		Models:  jsconv.Strings(m.Value("models")),
		Efforts: jsconv.Strings(m.Value("efforts")),
		MCP:     jsconv.Strings(m.Value("mcp")),
		Skills:  jsconv.Strings(m.Value("skills")),
		Tools:   jsconv.Strings(m.Value("tools")),
	}, true
}

// jsNumber is a close port of JavaScript Number(v) for decoded YAML values.
func jsNumber(v any) float64 {
	switch x := v.(type) {
	case nil:
		return 0
	case bool:
		if x {
			return 1
		}
		return 0
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case uint64:
		return float64(x)
	case float64:
		return x
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
		return math.NaN()
	default:
		return math.NaN()
	}
}

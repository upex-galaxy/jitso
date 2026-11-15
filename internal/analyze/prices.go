package analyze

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/upex-galaxy/jitso/internal/jsconv"
)

// Price is the price of one model family in USD per million tokens.
type Price struct {
	Input, CacheRead, CacheWrite5m, CacheWrite1h, Output float64
}

// Prices is the project's jitso/prices.json: a list-price estimate for
// comparing runs, never what was billed.
type Prices struct {
	// VerifiedOn is the JavaScript String() of the verified_on field.
	VerifiedOn string
	models     map[string]any
}

// TableLabel is the price table name an analyzed event records.
func (p Prices) TableLabel() string { return "jitso/prices@" + p.VerifiedOn }

// LoadPrices reads and parses the prices file. A missing or broken file is a
// plain error, which the CLI reports as exit 3.
func LoadPrices(path string) (Prices, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Prices{}, err
	}
	return ParsePrices(data)
}

// ParsePrices parses the text of a prices file.
func ParsePrices(data []byte) (Prices, error) {
	var top any
	if err := json.Unmarshal(data, &top); err != nil {
		return Prices{}, err
	}
	m, ok := top.(map[string]any)
	if !ok {
		return Prices{}, fmt.Errorf("the prices file is not a JSON object")
	}
	v, present := m["verified_on"]
	models, _ := m["models"].(map[string]any)
	return Prices{VerifiedOn: jsconv.String(jsconv.Of(v, present)), models: models}, nil
}

// For is the price of a family (sonnet, opus or haiku). A family the file
// does not price, or a price that is not a number, is an error: the oracle
// throws there, and a silent zero would hide a wrong cost.
func (p Prices) For(family string) (Price, error) {
	entry, ok := p.models[family].(map[string]any)
	if !ok {
		return Price{}, fmt.Errorf("the prices file has no entry for the %s family", family)
	}
	var out Price
	for _, field := range []struct {
		key string
		dst *float64
	}{
		{"input", &out.Input}, {"cache_read", &out.CacheRead}, {"cache_write_5m", &out.CacheWrite5m},
		{"cache_write_1h", &out.CacheWrite1h}, {"output", &out.Output},
	} {
		f, ok := entry[field.key].(float64)
		if !ok {
			return Price{}, fmt.Errorf("the %s price of the %s family is not a number", field.key, family)
		}
		*field.dst = f
	}
	return out, nil
}

// family maps a model name to its price family: opus and haiku by substring
// (case-insensitive), anything else, a missing name included, is sonnet.
func family(model any) string {
	name, _ := model.(string)
	name = strings.ToLower(name)
	switch {
	case strings.Contains(name, "opus"):
		return "opus"
	case strings.Contains(name, "haiku"):
		return "haiku"
	}
	return "sonnet"
}

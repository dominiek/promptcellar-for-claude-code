// Package pricing computes Anthropic-API cost estimates for a token bundle.
//
// Numbers below are best-effort estimates as of 2026-04. They WILL drift as
// Anthropic adjusts pricing. `pricingTable` is the single source of truth;
// update it in one place and rebuild. When the running model is not in the
// table, ComputeUSD returns 0 (which the PLF marshaller will then omit).
//
// Units throughout are USD per 1,000,000 tokens.
package pricing

type Tier struct {
	InputPerM      float64
	OutputPerM     float64
	CacheReadPerM  float64
	CacheWritePerM float64
}

// pricingTable maps the model name as it appears in PLF (`model.name`) to its
// per-million-token rates. Names mirror what Claude Code reports — both the
// canonical `claude-opus-4-7` and the context-tier-suffixed `[1m]` variants.
var pricingTable = map[string]Tier{
	"claude-opus-4-7":     {InputPerM: 15.00, OutputPerM: 75.00, CacheReadPerM: 1.50, CacheWritePerM: 18.75},
	"claude-opus-4-7[1m]": {InputPerM: 30.00, OutputPerM: 150.00, CacheReadPerM: 3.00, CacheWritePerM: 37.50},
	"claude-sonnet-4-6":   {InputPerM: 3.00, OutputPerM: 15.00, CacheReadPerM: 0.30, CacheWritePerM: 3.75},
	"claude-haiku-4-5":    {InputPerM: 1.00, OutputPerM: 5.00, CacheReadPerM: 0.10, CacheWritePerM: 1.25},
}

// ComputeUSD returns the dollar cost of a token bundle under the given model's
// pricing tier. Returns 0 if the model is not in the table.
func ComputeUSD(model string, input, output, cacheRead, cacheWrite int) float64 {
	t, ok := pricingTable[model]
	if !ok {
		return 0
	}
	const perToken = 1.0 / 1_000_000.0
	return float64(input)*t.InputPerM*perToken +
		float64(output)*t.OutputPerM*perToken +
		float64(cacheRead)*t.CacheReadPerM*perToken +
		float64(cacheWrite)*t.CacheWritePerM*perToken
}

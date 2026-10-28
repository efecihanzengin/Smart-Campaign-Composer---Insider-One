package llm

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// Mock is the scripted LLM used when no Anthropic API key is configured, and
// in tests. Each call runs its script; a nil script falls back to the default,
// which reads a few keywords from the Brief so the example Brief and its close
// variants get a convincing proposal.
type Mock struct {
	SegmentFunc func(ctx context.Context, req SegmentRequest) (json.RawMessage, error)
}

// NewMock returns a Mock running the default scripts.
func NewMock() *Mock {
	return &Mock{}
}

func (m *Mock) Segment(ctx context.Context, req SegmentRequest) (json.RawMessage, error) {
	if m.SegmentFunc != nil {
		return m.SegmentFunc(ctx, req)
	}
	return defaultSegment(req.Brief)
}

var (
	lastDays = regexp.MustCompile(`(?i)son (\d+) gün`)
	// Turkish city names as they appear in a Brief, possibly with a suffix
	// ("İstanbul'daki", "Ankara'da").
	knownCities = []string{"İstanbul", "Ankara", "İzmir", "Bursa", "Antalya", "Adana", "Konya",
		"Gaziantep", "Kocaeli", "Mersin", "Kayseri", "Eskişehir", "Diyarbakır", "Samsun", "Trabzon"}
)

// defaultSegment is the mock's stand-in for Claude's understanding of a Brief.
func defaultSegment(brief string) (json.RawMessage, error) {
	days := 30
	if m := lastDays.FindStringSubmatch(brief); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			days = n
		}
	}
	lower := strings.ToLower(brief)

	var conds []any
	for _, c := range knownCities {
		if strings.Contains(brief, c) {
			conds = append(conds, map[string]any{"field": "city", "op": "eq", "value": c})
			break
		}
	}
	if strings.Contains(lower, "sepet") {
		conds = append(conds, map[string]any{"event": "cart_add", "within_days": days, "has": true})
	}
	switch {
	case strings.Contains(lower, "alışveriş yapmayan"), strings.Contains(lower, "satın almayan"):
		conds = append(conds, map[string]any{"event": "purchase", "within_days": days, "has": false})
	case strings.Contains(lower, "alışveriş yapan"), strings.Contains(lower, "satın alan"):
		conds = append(conds, map[string]any{"event": "purchase", "within_days": days, "has": true})
	}

	unsupported := []string{}
	if len(conds) == 0 {
		// Nothing recognisable: fall back to recently active Recipients and say so.
		conds = append(conds, map[string]any{"event": "page_view", "within_days": 90, "has": true})
		unsupported = append(unsupported, "Brief'teki hedef kitle tarifi")
	}
	return json.Marshal(map[string]any{"rule": map[string]any{"all": conds}, "unsupported_criteria": unsupported})
}

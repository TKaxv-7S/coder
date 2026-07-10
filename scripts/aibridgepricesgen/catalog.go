package main

import (
	_ "embed"
	"encoding/json"
	"io"
	"time"

	"golang.org/x/xerrors"
)

// catalogJSON is the checked-in editorial curation input for the frontend
// known-models catalog. Entry order within each provider controls suggestion
// order in the UI. Everything factual (display name, limits, pricing,
// last_updated) is joined from models.dev at generation time; the curation
// file only carries editorial choices: which models to suggest, aliases,
// reasoning defaults, and overrides.
//
//go:embed catalog.json
var catalogJSON []byte

// curatedModel is one entry in catalog.json.
type curatedModel struct {
	ModelIdentifier string   `json:"modelIdentifier"`
	Aliases         []string `json:"aliases"`
	// DisplayName overrides the upstream `name` when set. Needed where
	// upstream naming does not match what we want to show (for example
	// "Claude Haiku 4.5 (latest)"), and for mirrored models.
	DisplayName string `json:"displayName"`
	// ReasoningEffort is editorial, not from models.dev. Mutually
	// exclusive with ThinkingBudgetTokens.
	ReasoningEffort string `json:"reasoningEffort"`
	// ThinkingBudgetTokens is Anthropic-only, for models that do not
	// support adaptive thinking and use the legacy
	// `thinking.budget_tokens` API instead.
	ThinkingBudgetTokens int `json:"thinkingBudgetTokens"`
	// MirrorOf names another upstream model whose specs and pricing this
	// entry copies. Used for models absent from models.dev (for example
	// claude-mythos-5, which Anthropic documents as sharing
	// claude-fable-5's specs and pricing).
	MirrorOf string `json:"mirrorOf"`
}

// catalogEntry matches the frontend KnownModel shape (knownModels/types.ts).
// Costs are flat USD per million tokens, straight from models.dev; tiered
// pricing such as context_over_200k is intentionally omitted.
type catalogEntry struct {
	Provider             string          `json:"provider"`
	ModelIdentifier      string          `json:"modelIdentifier"`
	DisplayName          string          `json:"displayName"`
	Aliases              []string        `json:"aliases"`
	ContextLimit         *int64          `json:"contextLimit,omitempty"`
	MaxOutputTokens      *int64          `json:"maxOutputTokens,omitempty"`
	ReasoningEffort      string          `json:"reasoningEffort,omitempty"`
	ThinkingBudgetTokens int             `json:"thinkingBudgetTokens,omitempty"`
	InputCost            *float64        `json:"inputCost,omitempty"`
	OutputCost           *float64        `json:"outputCost,omitempty"`
	CacheReadCost        *float64        `json:"cacheReadCost,omitempty"`
	CacheWriteCost       *float64        `json:"cacheWriteCost,omitempty"`
	SourceMetadata       catalogMetadata `json:"sourceMetadata"`
}

type catalogMetadata struct {
	SourceName        string `json:"sourceName"`
	SourceRetrievedAt string `json:"sourceRetrievedAt"`
	LastUpdated       string `json:"lastUpdated"`
}

// buildCatalog joins the curation file with the upstream models.dev payload
// and returns provider-keyed ordered entry lists. now supplies the
// sourceRetrievedAt date so output is deterministic under test.
func buildCatalog(upstream map[string]upstreamProvider, curation map[string][]curatedModel, now time.Time) (map[string][]catalogEntry, error) {
	retrievedAt := now.UTC().Format("2006-01-02")
	out := make(map[string][]catalogEntry, len(curation))
	for providerID, curated := range curation {
		provider, ok := upstream[providerID]
		if !ok {
			return nil, xerrors.Errorf("provider %q missing from upstream", providerID)
		}
		entries := make([]catalogEntry, 0, len(curated))
		for _, c := range curated {
			if c.ModelIdentifier == "" {
				return nil, xerrors.Errorf("provider %q: entry with empty modelIdentifier", providerID)
			}
			if c.ReasoningEffort != "" && c.ThinkingBudgetTokens != 0 {
				return nil, xerrors.Errorf("%s/%s: reasoningEffort and thinkingBudgetTokens are mutually exclusive", providerID, c.ModelIdentifier)
			}
			sourceID := c.ModelIdentifier
			if c.MirrorOf != "" {
				sourceID = c.MirrorOf
			}
			m, ok := provider.Models[sourceID]
			if !ok {
				if c.MirrorOf != "" {
					return nil, xerrors.Errorf("%s/%s: mirrorOf target %q missing from upstream", providerID, c.ModelIdentifier, c.MirrorOf)
				}
				return nil, xerrors.Errorf("%s/%s: model missing from upstream (use mirrorOf if intentional)", providerID, c.ModelIdentifier)
			}
			if !m.Cost.hasPricing() {
				return nil, xerrors.Errorf("%s/%s: upstream model %q has no cost block", providerID, c.ModelIdentifier, sourceID)
			}
			displayName := c.DisplayName
			if displayName == "" {
				displayName = m.Name
			}
			if displayName == "" {
				return nil, xerrors.Errorf("%s/%s: no displayName override and upstream name is empty", providerID, c.ModelIdentifier)
			}
			aliases := c.Aliases
			if aliases == nil {
				aliases = []string{}
			}
			entries = append(entries, catalogEntry{
				Provider:             providerID,
				ModelIdentifier:      c.ModelIdentifier,
				DisplayName:          displayName,
				Aliases:              aliases,
				ContextLimit:         m.Limit.Context,
				MaxOutputTokens:      m.Limit.Output,
				ReasoningEffort:      c.ReasoningEffort,
				ThinkingBudgetTokens: c.ThinkingBudgetTokens,
				InputCost:            m.Cost.Input,
				OutputCost:           m.Cost.Output,
				CacheReadCost:        m.Cost.CacheRead,
				CacheWriteCost:       m.Cost.CacheWrite,
				SourceMetadata: catalogMetadata{
					SourceName:        "models.dev",
					SourceRetrievedAt: retrievedAt,
					LastUpdated:       m.LastUpdated,
				},
			})
		}
		out[providerID] = entries
	}
	return out, nil
}

func writeCatalog(w io.Writer, catalog map[string][]catalogEntry) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(catalog); err != nil {
		return xerrors.Errorf("encode: %w", err)
	}
	return nil
}

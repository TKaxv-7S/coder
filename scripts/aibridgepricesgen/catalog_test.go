package main

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fixtureUpstream returns a small upstream payload covering the join cases:
// a fully priced model with limits, a mirror target, and a costless model.
func fixtureUpstream(t *testing.T) map[string]upstreamProvider {
	t.Helper()
	const upstreamJSON = `{
		"anthropic": {
			"models": {
				"claude-fable-5": {
					"name": "Claude Fable 5",
					"limit": {"context": 1000000, "output": 128000},
					"cost": {"input": 10, "output": 50, "cache_read": 1, "cache_write": 12.5},
					"last_updated": "2026-06-09"
				},
				"claude-costless": {
					"name": "Claude Costless",
					"limit": {"context": 200000, "output": 64000},
					"last_updated": "2026-01-01"
				}
			}
		},
		"openai": {
			"models": {
				"gpt-5.6-sol": {
					"name": "GPT-5.6 Sol",
					"limit": {"context": 1050000, "output": 128000},
					"cost": {"input": 5, "output": 30, "cache_read": 0.5, "cache_write": 6.25},
					"last_updated": "2026-07-09"
				},
				"gpt-partial": {
					"name": "GPT Partial",
					"limit": {"context": 400000, "output": 128000},
					"cost": {"input": 0.2, "output": 1.25},
					"last_updated": "2026-03-17"
				}
			}
		}
	}`
	var upstream map[string]upstreamProvider
	require.NoError(t, json.Unmarshal([]byte(upstreamJSON), &upstream))
	return upstream
}

var fixedNow = time.Date(2026, 7, 10, 12, 34, 56, 0, time.UTC)

func TestBuildCatalog(t *testing.T) {
	t.Parallel()

	curation := map[string][]curatedModel{
		"openai": {
			{ModelIdentifier: "gpt-5.6-sol", Aliases: []string{"gpt-5.6"}, ReasoningEffort: "medium"},
			{ModelIdentifier: "gpt-partial"},
		},
		"anthropic": {
			{ModelIdentifier: "claude-fable-5", ReasoningEffort: "high"},
			{ModelIdentifier: "claude-mythos-5", MirrorOf: "claude-fable-5", DisplayName: "Claude Mythos 5", ReasoningEffort: "high"},
		},
	}

	catalog, err := buildCatalog(fixtureUpstream(t), curation, fixedNow)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, writeCatalog(&buf, catalog))

	const want = `{
  "anthropic": [
    {
      "provider": "anthropic",
      "modelIdentifier": "claude-fable-5",
      "displayName": "Claude Fable 5",
      "aliases": [],
      "contextLimit": 1000000,
      "maxOutputTokens": 128000,
      "reasoningEffort": "high",
      "inputCost": 10,
      "outputCost": 50,
      "cacheReadCost": 1,
      "cacheWriteCost": 12.5,
      "sourceMetadata": {
        "sourceName": "models.dev",
        "sourceRetrievedAt": "2026-07-10",
        "lastUpdated": "2026-06-09"
      }
    },
    {
      "provider": "anthropic",
      "modelIdentifier": "claude-mythos-5",
      "displayName": "Claude Mythos 5",
      "aliases": [],
      "contextLimit": 1000000,
      "maxOutputTokens": 128000,
      "reasoningEffort": "high",
      "inputCost": 10,
      "outputCost": 50,
      "cacheReadCost": 1,
      "cacheWriteCost": 12.5,
      "sourceMetadata": {
        "sourceName": "models.dev",
        "sourceRetrievedAt": "2026-07-10",
        "lastUpdated": "2026-06-09"
      }
    }
  ],
  "openai": [
    {
      "provider": "openai",
      "modelIdentifier": "gpt-5.6-sol",
      "displayName": "GPT-5.6 Sol",
      "aliases": [
        "gpt-5.6"
      ],
      "contextLimit": 1050000,
      "maxOutputTokens": 128000,
      "reasoningEffort": "medium",
      "inputCost": 5,
      "outputCost": 30,
      "cacheReadCost": 0.5,
      "cacheWriteCost": 6.25,
      "sourceMetadata": {
        "sourceName": "models.dev",
        "sourceRetrievedAt": "2026-07-10",
        "lastUpdated": "2026-07-09"
      }
    },
    {
      "provider": "openai",
      "modelIdentifier": "gpt-partial",
      "displayName": "GPT Partial",
      "aliases": [],
      "contextLimit": 400000,
      "maxOutputTokens": 128000,
      "inputCost": 0.2,
      "outputCost": 1.25,
      "sourceMetadata": {
        "sourceName": "models.dev",
        "sourceRetrievedAt": "2026-07-10",
        "lastUpdated": "2026-03-17"
      }
    }
  ]
}
`
	require.Equal(t, want, buf.String())
}

func TestBuildCatalogDeterministic(t *testing.T) {
	t.Parallel()

	curation := map[string][]curatedModel{
		"openai": {{ModelIdentifier: "gpt-5.6-sol"}},
	}
	a, err := buildCatalog(fixtureUpstream(t), curation, fixedNow)
	require.NoError(t, err)
	b, err := buildCatalog(fixtureUpstream(t), curation, fixedNow)
	require.NoError(t, err)
	require.Equal(t, a, b)

	// A different injected clock changes only sourceRetrievedAt.
	later, err := buildCatalog(fixtureUpstream(t), curation, fixedNow.AddDate(0, 0, 1))
	require.NoError(t, err)
	require.Equal(t, "2026-07-11", later["openai"][0].SourceMetadata.SourceRetrievedAt)
}

func TestBuildCatalogErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		curation map[string][]curatedModel
		wantErr  string
	}{
		{
			name: "MissingUpstreamModel",
			curation: map[string][]curatedModel{
				"openai": {{ModelIdentifier: "gpt-nonexistent"}},
			},
			wantErr: "missing from upstream",
		},
		{
			name: "DanglingMirrorOf",
			curation: map[string][]curatedModel{
				"anthropic": {{ModelIdentifier: "claude-mythos-5", MirrorOf: "claude-nonexistent"}},
			},
			wantErr: "mirrorOf target",
		},
		{
			name: "NoCostBlock",
			curation: map[string][]curatedModel{
				"anthropic": {{ModelIdentifier: "claude-costless"}},
			},
			wantErr: "no cost block",
		},
		{
			name: "EffortAndBudgetBothSet",
			curation: map[string][]curatedModel{
				"anthropic": {{ModelIdentifier: "claude-fable-5", ReasoningEffort: "high", ThinkingBudgetTokens: 8192}},
			},
			wantErr: "mutually exclusive",
		},
		{
			name: "MissingProvider",
			curation: map[string][]curatedModel{
				"google": {{ModelIdentifier: "gemini"}},
			},
			wantErr: "missing from upstream",
		},
		{
			name: "EmptyModelIdentifier",
			curation: map[string][]curatedModel{
				"openai": {{}},
			},
			wantErr: "empty modelIdentifier",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := buildCatalog(fixtureUpstream(t), tc.curation, fixedNow)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// TestEmbeddedCatalogParses guards the checked-in curation file itself:
// valid JSON, required fields present, and the effort/budget exclusivity
// holds without needing upstream data.
func TestEmbeddedCatalogParses(t *testing.T) {
	t.Parallel()

	var curation map[string][]curatedModel
	require.NoError(t, json.Unmarshal(catalogJSON, &curation))
	require.NotEmpty(t, curation)
	for providerID, entries := range curation {
		require.NotEmpty(t, entries, providerID)
		for _, c := range entries {
			require.NotEmpty(t, c.ModelIdentifier, providerID)
			require.False(t, c.ReasoningEffort != "" && c.ThinkingBudgetTokens != 0,
				"%s/%s sets both reasoningEffort and thinkingBudgetTokens", providerID, c.ModelIdentifier)
		}
	}
}

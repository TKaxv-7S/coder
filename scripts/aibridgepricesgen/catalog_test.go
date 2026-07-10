package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fixtureUpstream returns a small upstream payload covering the join cases:
// fully priced models with limits and a costless model.
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
				"claude-mythos-5": {
					"name": "Claude Mythos 5",
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
			{ModelIdentifier: "claude-mythos-5", ReasoningEffort: "high"},
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

// TestBuildCatalogClock asserts the injected clock feeds sourceRetrievedAt
// and leaves upstream-derived fields alone.
func TestBuildCatalogClock(t *testing.T) {
	t.Parallel()

	curation := map[string][]curatedModel{
		"openai": {{ModelIdentifier: "gpt-5.6-sol"}},
	}
	later, err := buildCatalog(fixtureUpstream(t), curation, fixedNow.AddDate(0, 0, 1))
	require.NoError(t, err)
	require.Equal(t, "2026-07-11", later["openai"][0].SourceMetadata.SourceRetrievedAt)
	require.Equal(t, "2026-07-09", later["openai"][0].SourceMetadata.LastUpdated)
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
			name: "InvalidReasoningEffort",
			curation: map[string][]curatedModel{
				"anthropic": {{ModelIdentifier: "claude-fable-5", ReasoningEffort: "maximum"}},
			},
			wantErr: "is not one of",
		},
		{
			name: "NegativeThinkingBudget",
			curation: map[string][]curatedModel{
				"anthropic": {{ModelIdentifier: "claude-fable-5", ThinkingBudgetTokens: -1}},
			},
			wantErr: "is negative",
		},
		{
			name: "DuplicateModelIdentifier",
			curation: map[string][]curatedModel{
				"anthropic": {
					{ModelIdentifier: "claude-fable-5"},
					{ModelIdentifier: "claude-fable-5"},
				},
			},
			wantErr: "duplicate modelIdentifier",
		},
		{
			name: "DuplicateAlias",
			curation: map[string][]curatedModel{
				"anthropic": {
					{ModelIdentifier: "claude-fable-5", Aliases: []string{"claude-latest"}},
					{ModelIdentifier: "claude-mythos-5", Aliases: []string{"claude-latest"}},
				},
			},
			wantErr: "declared more than once",
		},
		{
			name: "EmptyAlias",
			curation: map[string][]curatedModel{
				"anthropic": {{ModelIdentifier: "claude-fable-5", Aliases: []string{""}}},
			},
			wantErr: "empty-string alias",
		},
		{
			name: "AliasShadowsModelIdentifier",
			curation: map[string][]curatedModel{
				"anthropic": {
					{ModelIdentifier: "claude-fable-5", Aliases: []string{"claude-mythos-5"}},
					{ModelIdentifier: "claude-mythos-5"},
				},
			},
			wantErr: "duplicates a modelIdentifier",
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

// TestEmbeddedCurationBuilds runs the checked-in curation.json through
// buildCatalog against a synthetic upstream derived from the curation's own
// model identifiers, so every buildCatalog validation (present and future)
// automatically guards the checked-in file.
func TestEmbeddedCurationBuilds(t *testing.T) {
	t.Parallel()

	curation := embeddedCuration(t)
	require.NotEmpty(t, curation)

	price := 1.0
	limit := int64(100000)
	upstream := make(map[string]upstreamProvider, len(curation))
	for providerID, entries := range curation {
		models := make(map[string]upstreamModel, len(entries))
		for _, c := range entries {
			models[c.ModelIdentifier] = upstreamModel{
				Name:        "Synthetic " + c.ModelIdentifier,
				Limit:       upstreamLimit{Context: &limit, Output: &limit},
				Cost:        &upstreamCost{Input: &price, Output: &price},
				LastUpdated: "2026-01-01",
			}
		}
		upstream[providerID] = upstreamProvider{Models: models}
	}

	catalog, err := buildCatalog(upstream, curation, fixedNow)
	require.NoError(t, err)
	for providerID, entries := range catalog {
		require.NotEmpty(t, entries, providerID)
	}
}

// TestCurationMatchesGeneratedCatalog is a drift test: the editorial fields
// (per provider, in order) in the embedded curation.json must exactly match
// their projection in the checked-in generated frontend catalog. Fails when
// curation.json changes without running `make gen/aibridge-prices`.
func TestCurationMatchesGeneratedCatalog(t *testing.T) {
	t.Parallel()

	curation := embeddedCuration(t)

	data, err := os.ReadFile("../../site/src/pages/AgentsPage/components/ChatModelAdminPanel/knownModels/knownModelsGenerated.json")
	require.NoError(t, err)
	var generated map[string][]catalogEntry
	require.NoError(t, json.Unmarshal(data, &generated))

	// editorial is the curation-owned projection of an entry. displayName is
	// only compared when the curation sets an override; otherwise it comes
	// from upstream and is not the curation's to pin.
	type editorial struct {
		ModelIdentifier      string
		Aliases              []string
		DisplayName          string
		ReasoningEffort      string
		ThinkingBudgetTokens int
	}

	curatedProjection := make(map[string][]editorial, len(curation))
	for providerID, entries := range curation {
		projected := make([]editorial, 0, len(entries))
		for _, c := range entries {
			aliases := c.Aliases
			if aliases == nil {
				aliases = []string{}
			}
			projected = append(projected, editorial{
				ModelIdentifier:      c.ModelIdentifier,
				Aliases:              aliases,
				DisplayName:          c.DisplayName,
				ReasoningEffort:      c.ReasoningEffort,
				ThinkingBudgetTokens: c.ThinkingBudgetTokens,
			})
		}
		curatedProjection[providerID] = projected
	}

	generatedProjection := make(map[string][]editorial, len(generated))
	for providerID, entries := range generated {
		curated := map[string]curatedModel{}
		for _, c := range curation[providerID] {
			curated[c.ModelIdentifier] = c
		}
		projected := make([]editorial, 0, len(entries))
		for _, e := range entries {
			displayName := ""
			if curated[e.ModelIdentifier].DisplayName != "" {
				displayName = e.DisplayName
			}
			projected = append(projected, editorial{
				ModelIdentifier:      e.ModelIdentifier,
				Aliases:              e.Aliases,
				DisplayName:          displayName,
				ReasoningEffort:      e.ReasoningEffort,
				ThinkingBudgetTokens: e.ThinkingBudgetTokens,
			})
		}
		generatedProjection[providerID] = projected
	}

	require.Equal(t, curatedProjection, generatedProjection,
		"curation.json and knownModelsGenerated.json disagree; run `make gen/aibridge-prices`")
}

func embeddedCuration(t *testing.T) map[string][]curatedModel {
	t.Helper()
	var curation map[string][]curatedModel
	require.NoError(t, json.Unmarshal(curationJSON, &curation))
	return curation
}

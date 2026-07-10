import { describe, expect, it } from "vitest";
import knownModelsGenerated from "./knownModelsGenerated.json";

// knownModelsGenerated.json crosses a typed boundary via a cast in index.ts,
// so this suite validates the shape and enum values of every generated entry.
const providers = Object.entries(knownModelsGenerated);

describe("knownModelsGenerated", () => {
	it("contains the expected canonical model identifiers in display order", () => {
		expect(
			knownModelsGenerated.openai.map((model) => model.modelIdentifier),
		).toEqual([
			"gpt-5.6-sol",
			"gpt-5.6-terra",
			"gpt-5.6-luna",
			"gpt-5.5",
			"gpt-5.5-pro",
			"gpt-5.4",
			"gpt-5.4-mini",
			"gpt-5.4-nano",
			"gpt-5.3-codex",
		]);
		expect(
			knownModelsGenerated.anthropic.map((model) => model.modelIdentifier),
		).toEqual([
			"claude-fable-5",
			"claude-mythos-5",
			"claude-opus-4-8",
			"claude-opus-4-7",
			"claude-opus-4-6",
			"claude-sonnet-4-6",
			"claude-haiku-4-5",
			"claude-sonnet-4-5",
		]);
	});

	it.each(providers)("validates every %s entry", (provider, models) => {
		expect(models.length).toBeGreaterThan(0);
		for (const model of models) {
			expect(model.provider).toBe(provider);
			expect(model.modelIdentifier).not.toBe("");
			expect(model.displayName).not.toBe("");
			expect(Array.isArray(model.aliases)).toBe(true);

			const record = model as Record<string, unknown>;
			if (record.reasoningEffort !== undefined) {
				expect(["low", "medium", "high"]).toContain(record.reasoningEffort);
			}
			expect(
				record.reasoningEffort !== undefined &&
					record.thinkingBudgetTokens !== undefined,
			).toBe(false);

			for (const field of [
				"contextLimit",
				"maxOutputTokens",
				"thinkingBudgetTokens",
				"inputCost",
				"outputCost",
				"cacheReadCost",
				"cacheWriteCost",
			]) {
				const value = record[field];
				if (value !== undefined) {
					expect(typeof value, field).toBe("number");
					expect(value, field).toBeGreaterThan(0);
				}
			}

			expect(model.sourceMetadata.sourceName).toBe("models.dev");
			expect(model.sourceMetadata.sourceRetrievedAt).not.toBe("");
			expect(model.sourceMetadata.lastUpdated).not.toBe("");
		}
	});
});

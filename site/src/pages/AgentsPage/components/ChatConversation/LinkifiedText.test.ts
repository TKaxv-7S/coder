import { describe, expect, it } from "vitest";
import { tokenizeLinkifiedText } from "./LinkifiedText";

describe("tokenizeLinkifiedText", () => {
	it.each([
		{
			name: "links an HTTPS URL in a sentence",
			text: "Visit https://coder.com/docs for details",
			expected: [
				{ type: "text", value: "Visit " },
				{
					type: "url",
					value: "https://coder.com/docs",
					href: "https://coder.com/docs",
				},
				{ type: "text", value: " for details" },
			],
		},
		{
			name: "links an HTTP URL",
			text: "http://localhost:3000/path",
			expected: [
				{
					type: "url",
					value: "http://localhost:3000/path",
					href: "http://localhost:3000/path",
				},
			],
		},
		{
			name: "adds an HTTPS scheme to a www URL",
			text: "www.coder.com",
			expected: [
				{
					type: "url",
					value: "www.coder.com",
					href: "https://www.coder.com",
				},
			],
		},
		{
			name: "excludes trailing punctuation",
			text: "See https://coder.com/docs., and www.coder.com?",
			expected: [
				{ type: "text", value: "See " },
				{
					type: "url",
					value: "https://coder.com/docs",
					href: "https://coder.com/docs",
				},
				{ type: "text", value: "., and " },
				{
					type: "url",
					value: "www.coder.com",
					href: "https://www.coder.com",
				},
				{ type: "text", value: "?" },
			],
		},
		{
			name: "keeps balanced URL parentheses",
			text: "https://en.wikipedia.org/wiki/Foo_(bar)",
			expected: [
				{
					type: "url",
					value: "https://en.wikipedia.org/wiki/Foo_(bar)",
					href: "https://en.wikipedia.org/wiki/Foo_(bar)",
				},
			],
		},
		{
			name: "excludes an unmatched trailing parenthesis",
			text: "(https://coder.com/docs)",
			expected: [
				{ type: "text", value: "(" },
				{
					type: "url",
					value: "https://coder.com/docs",
					href: "https://coder.com/docs",
				},
				{ type: "text", value: ")" },
			],
		},
		{
			name: "links multiple URLs",
			text: "https://coder.com and www.example.com",
			expected: [
				{
					type: "url",
					value: "https://coder.com",
					href: "https://coder.com",
				},
				{ type: "text", value: " and " },
				{
					type: "url",
					value: "www.example.com",
					href: "https://www.example.com",
				},
			],
		},
		{
			name: "preserves text without URLs",
			text: "Keep *markdown* literal",
			expected: [{ type: "text", value: "Keep *markdown* literal" }],
		},
		{
			name: "preserves newlines in text segments",
			text: "First line\nhttps://coder.com\nLast line",
			expected: [
				{ type: "text", value: "First line\n" },
				{
					type: "url",
					value: "https://coder.com",
					href: "https://coder.com",
				},
				{ type: "text", value: "\nLast line" },
			],
		},
		{
			name: "does not link a bare domain",
			text: "Visit coder.com",
			expected: [{ type: "text", value: "Visit coder.com" }],
		},
		{
			name: "does not link incomplete URL prefixes",
			text: "Keep https:// and www. literal",
			expected: [{ type: "text", value: "Keep https:// and www. literal" }],
		},
		{
			name: "handles empty text",
			text: "",
			expected: [],
		},
	])("$name", ({ text, expected }) => {
		expect(tokenizeLinkifiedText(text)).toEqual(expected);
	});
});

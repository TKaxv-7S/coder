import { type FC, Fragment } from "react";
import type { UrlTransform } from "streamdown";

const URL_PATTERN =
	/\b(?:https?:\/\/(?:[a-z0-9]|\[[0-9a-f:]+\])|www\.[a-z0-9])[^\s<>"']*/gi;
const TRAILING_PUNCTUATION = new Set([".", ",", ";", ":", "!", "?"]);

type LinkifiedTextSegment =
	| { type: "text"; value: string }
	| { type: "url"; value: string; href: string };

const trimURLSuffix = (value: string) => {
	let end = value.length;
	let openParentheses = 0;
	let closeParentheses = 0;

	for (const character of value) {
		if (character === "(") {
			openParentheses++;
		} else if (character === ")") {
			closeParentheses++;
		}
	}

	while (end > 0) {
		const character = value[end - 1];
		if (TRAILING_PUNCTUATION.has(character)) {
			end--;
			continue;
		}
		if (character === ")" && closeParentheses > openParentheses) {
			end--;
			closeParentheses--;
			continue;
		}
		break;
	}

	return value.slice(0, end);
};

/** Splits literal prompt text into text and safe HTTP(S) URL segments. */
export const tokenizeLinkifiedText = (text: string): LinkifiedTextSegment[] => {
	const segments: LinkifiedTextSegment[] = [];
	let cursor = 0;

	for (const match of text.matchAll(URL_PATTERN)) {
		const index = match.index;
		const value = trimURLSuffix(match[0]);
		if (value.length === 0) {
			continue;
		}

		if (index > cursor) {
			segments.push({ type: "text", value: text.slice(cursor, index) });
		}
		segments.push({
			type: "url",
			value,
			href: value.toLowerCase().startsWith("www.") ? `https://${value}` : value,
		});
		cursor = index + value.length;
	}

	if (cursor < text.length) {
		segments.push({ type: "text", value: text.slice(cursor) });
	}

	return segments;
};

export const LinkifiedText: FC<{
	text: string;
	urlTransform?: UrlTransform;
}> = ({ text, urlTransform }) => {
	return tokenizeLinkifiedText(text).map((segment, index) => {
		if (segment.type === "text") {
			return <Fragment key={index}>{segment.value}</Fragment>;
		}

		const href =
			(urlTransform
				? urlTransform(segment.href, "href", {
						type: "element",
						tagName: "a",
						properties: { href: segment.href },
						children: [{ type: "text", value: segment.value }],
					})
				: segment.href) ?? segment.href;
		return (
			<a
				key={index}
				href={href}
				target="_blank"
				rel="noopener noreferrer"
				className="text-content-link no-underline hover:underline hover:decoration-content-link"
			>
				{segment.value}
			</a>
		);
	});
};

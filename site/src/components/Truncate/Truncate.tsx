import {
	type FC,
	type HTMLAttributes,
	useCallback,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { cn } from "#/utils/cn";

type TruncatePosition = "start" | "middle" | "end";

type TruncateProps = Omit<
	HTMLAttributes<HTMLSpanElement>,
	"children" | "title"
> & {
	children: string;
	position?: TruncatePosition;
	ellipsis?: string;
	/**
	 * Tooltip shown on hover. Defaults to the full, untruncated value so the
	 * complete string is always recoverable. Pass `false` to omit the title.
	 */
	title?: string | false;
};

// Split into grapheme clusters so multi-code-unit characters (emoji,
// combining marks) are never cut in half when truncating.
const toGraphemes = (value: string): string[] => {
	if ("Segmenter" in Intl) {
		const segmenter = new Intl.Segmenter(undefined, {
			granularity: "grapheme",
		});
		return Array.from(segmenter.segment(value), ({ segment }) => segment);
	}
	return Array.from(value);
};

const buildCandidate = (
	graphemes: string[],
	kept: number,
	position: TruncatePosition,
	ellipsis: string,
): string => {
	switch (position) {
		case "start":
			return ellipsis + graphemes.slice(graphemes.length - kept).join("");
		case "end":
			return graphemes.slice(0, kept).join("") + ellipsis;
		case "middle": {
			const startLength = Math.ceil(kept / 2);
			const endLength = Math.floor(kept / 2);
			const start = graphemes.slice(0, startLength).join("");
			const end =
				endLength > 0
					? graphemes.slice(graphemes.length - endLength).join("")
					: "";
			return `${start}${ellipsis}${end}`;
		}
	}
};

/**
 * Truncates a single line of text to fit its container, inserting an ellipsis
 * at the start, middle, or end. Unlike the CSS `truncate` utility, middle
 * truncation keeps both ends of the string visible, which matters for file
 * paths, URLs, and opaque identifiers where the leading and trailing
 * characters both carry meaning.
 *
 * The visible text is measured against the rendered container width, so it
 * reflows on container resize and when web fonts finish loading. The full
 * value is exposed on hover through the `title` attribute (pass `title={false}`
 * to opt out).
 *
 * The container fills its parent's width, so constrain it through the parent
 * (for example a flex item with `min-w-0`, or a fixed-width cell).
 */
export const Truncate: FC<TruncateProps> = ({
	children,
	position = "end",
	ellipsis = "…",
	title = children,
	className,
	...props
}) => {
	const containerRef = useRef<HTMLSpanElement>(null);
	const measurementRef = useRef<HTMLSpanElement>(null);
	const [displayValue, setDisplayValue] = useState(children);

	const recalculate = useCallback(() => {
		const container = containerRef.current;
		const measurement = measurementRef.current;
		if (!container || !measurement) {
			return;
		}

		try {
			const computedStyle = getComputedStyle(container);
			const horizontalPadding =
				Number.parseFloat(computedStyle.paddingLeft) +
				Number.parseFloat(computedStyle.paddingRight);
			const availableWidth = container.clientWidth - horizontalPadding;

			const fits = (value: string): boolean => {
				measurement.textContent = value;
				// Allow half a pixel of slack to absorb sub-pixel rounding.
				return (
					measurement.getBoundingClientRect().width <= availableWidth + 0.5
				);
			};

			if (fits(children)) {
				setDisplayValue(children);
				return;
			}
			if (!fits(ellipsis)) {
				setDisplayValue("");
				return;
			}

			// Binary search for the largest grapheme count that still fits.
			const graphemes = toGraphemes(children);
			let minimum = 0;
			let maximum = graphemes.length;
			let bestMatch = ellipsis;
			while (minimum <= maximum) {
				const kept = Math.floor((minimum + maximum) / 2);
				const candidate = buildCandidate(graphemes, kept, position, ellipsis);
				if (fits(candidate)) {
					bestMatch = candidate;
					minimum = kept + 1;
				} else {
					maximum = kept - 1;
				}
			}
			setDisplayValue(bestMatch);
		} finally {
			// The measurement span sits in the DOM alongside the visible text.
			// Leaving the last-tested value in it would duplicate the string in
			// `textContent`, corrupting copy-paste and confusing text queries.
			measurement.textContent = "";
		}
	}, [children, ellipsis, position]);

	useLayoutEffect(() => {
		const container = containerRef.current;
		if (!container) {
			return;
		}
		const resizeObserver = new ResizeObserver(recalculate);
		resizeObserver.observe(container);
		document.fonts?.addEventListener("loadingdone", recalculate);
		recalculate();
		return () => {
			resizeObserver.disconnect();
			document.fonts?.removeEventListener("loadingdone", recalculate);
		};
	}, [recalculate]);

	return (
		<span
			{...props}
			ref={containerRef}
			title={title === false ? undefined : title}
			className={cn(
				"relative block w-full min-w-0 overflow-hidden whitespace-nowrap",
				className,
			)}
		>
			{displayValue}
			<span
				ref={measurementRef}
				aria-hidden
				className="pointer-events-none invisible absolute w-max whitespace-pre"
			/>
		</span>
	);
};

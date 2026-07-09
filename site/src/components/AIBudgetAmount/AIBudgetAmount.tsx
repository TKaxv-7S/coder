import { cva } from "class-variance-authority";
import type { FC } from "react";
import { getSeverity } from "#/utils/budget";
import { formatBudgetUSD } from "#/utils/currency";

const amountVariants = cva("", {
	variants: {
		severity: {
			normal: "text-content-primary",
			warning: "text-content-warning",
			exceeded: "text-content-destructive",
		},
	},
});

/** A spend amount in USD that takes the warning/exceeded color as it nears the limit; values in micros. */
export const AIBudgetAmount: FC<{ spend: number; limit: number }> = ({
	spend,
	limit,
}) => (
	<span className={amountVariants({ severity: getSeverity(spend, limit) })}>
		{formatBudgetUSD(spend)}
	</span>
);

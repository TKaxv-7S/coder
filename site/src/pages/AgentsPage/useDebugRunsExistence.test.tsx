import { renderHook, waitFor } from "@testing-library/react";
import type { FC, PropsWithChildren } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { ChatDebugRunSummary } from "#/api/typesGenerated";
import { useDebugRunsExistence } from "./useDebugRunsExistence";

const CHAT_ID = "chat-1";

const MockDebugRunSummary: ChatDebugRunSummary = {
	id: "run-1",
	chat_id: CHAT_ID,
	kind: "chat_turn",
	status: "error",
	summary: {},
	started_at: "2024-01-01T00:00:00Z",
	updated_at: "2024-01-01T00:00:01Z",
	finished_at: "2024-01-01T00:00:01Z",
};

const createWrapper = (queryClient: QueryClient): FC<PropsWithChildren> => {
	return ({ children }) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
};

describe("useDebugRunsExistence", () => {
	let queryClient: QueryClient;

	beforeEach(() => {
		queryClient = new QueryClient({
			defaultOptions: { queries: { retry: false } },
		});
		vi.restoreAllMocks();
	});

	it("fetches once on mount and reports no runs", async () => {
		const getChatDebugRuns = vi
			.spyOn(API.experimental, "getChatDebugRuns")
			.mockResolvedValue([]);

		const { result } = renderHook(() => useDebugRunsExistence(CHAT_ID, false), {
			wrapper: createWrapper(queryClient),
		});

		await waitFor(() => expect(result.current.hasDebugRuns).toBe(false));
		expect(getChatDebugRuns).toHaveBeenCalledTimes(1);
	});

	it("does not poll while a turn is in flight", async () => {
		const getChatDebugRuns = vi
			.spyOn(API.experimental, "getChatDebugRuns")
			.mockResolvedValue([]);

		const { result, rerender } = renderHook(
			({ chatTurnInFlight }) =>
				useDebugRunsExistence(CHAT_ID, chatTurnInFlight),
			{
				wrapper: createWrapper(queryClient),
				initialProps: { chatTurnInFlight: true },
			},
		);

		await waitFor(() => expect(getChatDebugRuns).toHaveBeenCalledTimes(1));

		// Re-rendering with the turn still in flight must not trigger any
		// additional fetch: there is no polling loop.
		rerender({ chatTurnInFlight: true });
		rerender({ chatTurnInFlight: true });
		expect(getChatDebugRuns).toHaveBeenCalledTimes(1);
		expect(result.current.hasDebugRuns).toBe(false);
	});

	it("refetches exactly once when the turn transitions from in-flight to terminal", async () => {
		const getChatDebugRuns = vi
			.spyOn(API.experimental, "getChatDebugRuns")
			.mockResolvedValueOnce([])
			.mockResolvedValueOnce([MockDebugRunSummary]);

		const { result, rerender } = renderHook(
			({ chatTurnInFlight }) =>
				useDebugRunsExistence(CHAT_ID, chatTurnInFlight),
			{
				wrapper: createWrapper(queryClient),
				initialProps: { chatTurnInFlight: true },
			},
		);

		await waitFor(() => expect(getChatDebugRuns).toHaveBeenCalledTimes(1));
		expect(result.current.hasDebugRuns).toBe(false);

		// The turn ends: this is the only edge that should trigger a refetch.
		rerender({ chatTurnInFlight: false });
		await waitFor(() => expect(getChatDebugRuns).toHaveBeenCalledTimes(2));
		await waitFor(() => expect(result.current.hasDebugRuns).toBe(true));

		// Further re-renders in the terminal state must not refetch again.
		rerender({ chatTurnInFlight: false });
		expect(getChatDebugRuns).toHaveBeenCalledTimes(2);
	});

	it("does not refetch on the terminal edge once a run is already known", async () => {
		const getChatDebugRuns = vi
			.spyOn(API.experimental, "getChatDebugRuns")
			.mockResolvedValue([MockDebugRunSummary]);

		const { result, rerender } = renderHook(
			({ chatTurnInFlight }) =>
				useDebugRunsExistence(CHAT_ID, chatTurnInFlight),
			{
				wrapper: createWrapper(queryClient),
				initialProps: { chatTurnInFlight: true },
			},
		);

		await waitFor(() => expect(result.current.hasDebugRuns).toBe(true));
		expect(getChatDebugRuns).toHaveBeenCalledTimes(1);

		rerender({ chatTurnInFlight: false });
		expect(getChatDebugRuns).toHaveBeenCalledTimes(1);
	});
});

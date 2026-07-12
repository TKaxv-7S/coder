CREATE TABLE workspace_agent_session_counts (
	workspace_agent_stats_id uuid NOT NULL REFERENCES workspace_agent_stats (id) ON DELETE CASCADE,
	app_name text NOT NULL,
	count bigint NOT NULL DEFAULT 0,
	PRIMARY KEY (workspace_agent_stats_id, app_name)
);

COMMENT ON TABLE workspace_agent_session_counts IS 'Session counts per app for each workspace agent stats row. Rows are removed together with their parent workspace_agent_stats row.';
COMMENT ON COLUMN workspace_agent_session_counts.app_name IS 'App identifier as reported by the client (e.g. vscode, jetbrains, ssh, reconnecting_pty), canonicalized at ingestion: lowercased with hyphens folded to underscores. Family grouping is applied at read time.';

-- Copy the ephemeral buffer (~1 day of rows) into the new table so that
-- template usage rollup and deployment stats see no gap during the upgrade.
INSERT INTO workspace_agent_session_counts (workspace_agent_stats_id, app_name, count)
SELECT s.id, v.app_name, v.count
FROM workspace_agent_stats s
CROSS JOIN LATERAL (
	VALUES
		('vscode', s.session_count_vscode),
		('jetbrains', s.session_count_jetbrains),
		('reconnecting_pty', s.session_count_reconnecting_pty),
		('ssh', s.session_count_ssh)
) v(app_name, count)
WHERE v.count > 0;

DROP INDEX workspace_agent_stats_template_id_created_at_user_id_idx;

ALTER TABLE workspace_agent_stats
	DROP COLUMN session_count_vscode,
	DROP COLUMN session_count_jetbrains,
	DROP COLUMN session_count_reconnecting_pty,
	DROP COLUMN session_count_ssh;

CREATE INDEX workspace_agent_stats_template_id_created_at_user_id_idx ON workspace_agent_stats USING btree (template_id, created_at, user_id) INCLUDE (connection_median_latency_ms) WHERE (connection_count > 0);

COMMENT ON INDEX workspace_agent_stats_template_id_created_at_user_id_idx IS 'Support index for template insights endpoint to build interval reports faster.';

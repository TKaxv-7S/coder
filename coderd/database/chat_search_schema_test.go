package database_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

func TestChatMessageSearchText(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	_, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

	cases := []struct {
		name    string
		content sql.NullString // JSONB input; invalid means SQL NULL.
		want    sql.NullString // expected text; invalid means SQL NULL.
	}{
		{
			name:    "SingleTextPart",
			content: sql.NullString{String: `[{"type":"text","text":"hello world"}]`, Valid: true},
			want:    sql.NullString{String: "hello world", Valid: true},
		},
		{
			name: "TextInterleavedWithNonText",
			content: sql.NullString{String: `[
				{"type":"text","text":"first"},
				{"type":"reasoning","text":"thinking"},
				{"type":"tool-call","toolName":"execute"},
				{"type":"text","text":"second"}
			]`, Valid: true},
			want: sql.NullString{String: "first second", Valid: true},
		},
		{
			name:    "OnlyNonTextParts",
			content: sql.NullString{String: `[{"type":"reasoning","text":"thinking"}]`, Valid: true},
			want:    sql.NullString{},
		},
		{
			name:    "ScalarContent",
			content: sql.NullString{String: `"hello"`, Valid: true},
			want:    sql.NullString{},
		},
		{
			name:    "EmptyArray",
			content: sql.NullString{String: `[]`, Valid: true},
			want:    sql.NullString{},
		},
		{
			name:    "NullInput",
			content: sql.NullString{},
			want:    sql.NullString{},
		},
		{
			name:    "ElementsMissingTypeOrText",
			content: sql.NullString{String: `[{"text":"no type"},{"type":"text"},{"type":"text","text":"kept"}]`, Valid: true},
			want:    sql.NullString{String: "kept", Valid: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitMedium)
			var got sql.NullString
			err := sqlDB.QueryRowContext(ctx,
				`SELECT chat_message_search_text($1::jsonb)`, tc.content,
			).Scan(&got)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// eligibilityPredicate is the shared eligibility portion of the two partial
// chat_messages search indexes. Queries must repeat it verbatim to use them.
const eligibilityPredicate = `deleted = false
	AND visibility IN ('user', 'both')
	AND role IN ('user', 'assistant')`

func TestChatSearchSchemaIndexes(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	_, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitMedium)

	cases := []struct {
		name    string
		table   string
		partial bool
	}{
		{name: "idx_chat_messages_search_tsv", table: "chat_messages", partial: true},
		{name: "idx_chat_messages_search_tsv_pending", table: "chat_messages", partial: true},
		{name: "idx_chats_title_fts", table: "chats", partial: false},
		{name: "idx_chat_diff_statuses_pr_title_fts", table: "chat_diff_statuses", partial: false},
	}
	for _, tc := range cases {
		var table string
		var partial bool
		err := sqlDB.QueryRowContext(ctx, `
			SELECT i.tablename, x.indpred IS NOT NULL
			FROM pg_indexes i
			JOIN pg_class c ON c.relname = i.indexname
			JOIN pg_index x ON x.indexrelid = c.oid
			WHERE i.indexname = $1`, tc.name,
		).Scan(&table, &partial)
		require.NoError(t, err, "index %s should exist", tc.name)
		require.Equal(t, tc.table, table, "index %s table", tc.name)
		require.Equal(t, tc.partial, partial, "index %s partial", tc.name)
	}
}

func TestChatSearchSchemaBehavior(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	org := dbgen.Organization(t, db, database.Organization{})
	owner := dbgen.User(t, db, database.User{})
	_ = dbgen.ChatProvider(t, db, database.ChatProvider{Provider: "openai", DisplayName: "OpenAI"})
	modelCfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		CreatedBy: uuid.NullUUID{UUID: owner.ID, Valid: true},
		UpdatedBy: uuid.NullUUID{UUID: owner.ID, Valid: true},
		IsDefault: true,
	})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           owner.ID,
		LastModelConfigID: modelCfg.ID,
	})

	newMsg := func(role database.ChatMessageRole, visibility database.ChatMessageVisibility, content string) database.ChatMessage {
		seed := database.ChatMessage{
			ChatID:     chat.ID,
			CreatedBy:  uuid.NullUUID{UUID: owner.ID, Valid: true},
			Role:       role,
			Visibility: visibility,
		}
		if content != "" {
			seed.Content = pqtype.NullRawMessage{RawMessage: []byte(content), Valid: true}
		}
		return dbgen.ChatMessage(t, db, seed)
	}
	textContent := func(text string) string {
		return `[{"type":"text","text":"` + text + `"}]`
	}

	pendingIDs := func(ctx context.Context, limit int) []int64 {
		rows, err := sqlDB.QueryContext(ctx, `
			SELECT id FROM chat_messages
			WHERE search_tsv IS NULL AND `+eligibilityPredicate+`
			ORDER BY id DESC
			LIMIT $1`, limit)
		require.NoError(t, err)
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var id int64
			require.NoError(t, rows.Scan(&id))
			ids = append(ids, id)
		}
		require.NoError(t, rows.Err())
		return ids
	}

	// Insert regression: RETURNING * must survive the new column, and new
	// rows must start with search_tsv NULL so they enter the pending queue.
	eligibleText := newMsg(database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth, textContent("deploy the search feature"))
	var tsvIsNull bool
	err := sqlDB.QueryRowContext(ctx,
		`SELECT search_tsv IS NULL FROM chat_messages WHERE id = $1`, eligibleText.ID,
	).Scan(&tsvIsNull)
	require.NoError(t, err)
	require.True(t, tsvIsNull, "new rows must have search_tsv NULL")

	eligibleNoText := newMsg(database.ChatMessageRoleAssistant, database.ChatMessageVisibilityUser, `[{"type":"reasoning","text":"thinking"}]`)
	toolMsg := newMsg(database.ChatMessageRoleTool, database.ChatMessageVisibilityBoth, textContent("tool output about deploy"))
	modelOnly := newMsg(database.ChatMessageRoleUser, database.ChatMessageVisibilityModel, textContent("model-only deploy note"))
	deletedMsg := newMsg(database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth, textContent("deleted deploy message"))
	_, err = sqlDB.ExecContext(ctx, `UPDATE chat_messages SET deleted = true WHERE id = $1`, deletedMsg.ID)
	require.NoError(t, err)

	// Only eligible rows appear in the queue, newest first. The tool-role,
	// model-only, and soft-deleted rows are excluded even though their
	// search_tsv is NULL.
	require.Equal(t, []int64{eligibleNoText.ID, eligibleText.ID}, pendingIDs(ctx, 10))

	// Sweep-style UPDATE. The '' sentinel (not NULL) marks no-text rows as
	// swept; NULL means pending, so COALESCE is what drains them from the
	// queue.
	_, err = sqlDB.ExecContext(ctx, `
		UPDATE chat_messages
		SET search_tsv = COALESCE(to_tsvector('simple', chat_message_search_text(content)), ''::tsvector)
		WHERE id = ANY($1)`, pq.Array([]int64{eligibleText.ID, eligibleNoText.ID}))
	require.NoError(t, err)
	require.Empty(t, pendingIDs(ctx, 10), "swept rows must leave the queue, including no-text rows")

	// Soft-deleting an unswept row removes it from the queue without a sweep.
	unswept := newMsg(database.ChatMessageRoleUser, database.ChatMessageVisibilityBoth, textContent("unswept deploy row"))
	require.Equal(t, []int64{unswept.ID}, pendingIDs(ctx, 10))
	_, err = sqlDB.ExecContext(ctx, `UPDATE chat_messages SET deleted = true WHERE id = $1`, unswept.ID)
	require.NoError(t, err)
	require.Empty(t, pendingIDs(ctx, 10))

	// Search contract: populate search_tsv on every row (including
	// ineligible ones) and assert the search-index predicate filters them.
	_, err = sqlDB.ExecContext(ctx, `
		UPDATE chat_messages
		SET search_tsv = COALESCE(to_tsvector('simple', chat_message_search_text(content)), ''::tsvector)
		WHERE chat_id = $1`, chat.ID)
	require.NoError(t, err)

	rows, err := sqlDB.QueryContext(ctx, `
		SELECT id FROM chat_messages
		WHERE search_tsv @@ websearch_to_tsquery('simple', $1)
			AND search_tsv IS NOT NULL
			AND `+eligibilityPredicate+`
		ORDER BY id`, "deploy")
	require.NoError(t, err)
	defer rows.Close()
	var matched []int64
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		matched = append(matched, id)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []int64{eligibleText.ID}, matched,
		"search must exclude deleted, model-only, and tool-role rows (%d %d %d)",
		toolMsg.ID, modelOnly.ID, deletedMsg.ID)
}

-- CASE guard: jsonb_array_elements raises on non-array input. Legacy
-- content_version=0 rows store scalar JSON strings; excluded from search
-- by design. IMMUTABLE for expression index use.
CREATE FUNCTION chat_message_search_text(content jsonb) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT CASE WHEN jsonb_typeof(content) = 'array' THEN (
        SELECT string_agg(part->>'text', ' ' ORDER BY ordinality)
        FROM jsonb_array_elements(content) WITH ORDINALITY AS t(part, ordinality)
        WHERE part->>'type' = 'text'
    ) END
$$;

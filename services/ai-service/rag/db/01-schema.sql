-- =============================================================================
-- Book Recommender RAG — schema
-- Target: Postgres 17 + pgvector, running in container `n8n-book-rag-db`.
--
-- This file is executed automatically on FIRST start of the container, because
-- docker-compose.yml mounts ./db into /docker-entrypoint-initdb.d.
-- To apply it manually later:
--   docker exec -i n8n-book-rag-db psql -U bookrag -d bookrag < db/01-schema.sql
--
-- EVERY statement here is idempotent, so re-running is safe. This is a deliberate
-- correction of the template this project replaced, which used bare CREATE TABLE
-- and therefore failed permanently on the second run (see docs/RULES.md §2.3).
--
-- EMBEDDING_DIM = 3072. This is a project constant of record (docs/DECISIONS.md
-- ADR-002). It MUST match the dimension the embeddings node emits, or every
-- insert fails with a dimension mismatch.
--
-- WHY 3072 AND NOT 1536: n8n's "Embeddings Google Gemini" node exposes NO
-- output-dimension control (verified against the node source), and both
-- gemini-embedding-001 and gemini-embedding-2 emit 3072 dimensions by default
-- (verified against Google's embeddings documentation). 3072 is therefore what
-- the workflow actually sends, so 3072 is what the column must accept.
-- Do not "optimise" this to 1536 without first proving the node can emit 1536.
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS vector;

-- -----------------------------------------------------------------------------
-- books — the catalogue. One row per book, independent of chunking, so that
-- re-chunking a book never destroys its bibliographic record.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS books (
    book_id        text PRIMARY KEY,
    title          text NOT NULL,
    author         text,
    genre          text,
    tags           text[]      NOT NULL DEFAULT '{}',
    published_year int,
    page_count     int,
    language       text        DEFAULT 'en',
    rating         numeric(3,2),
    description    text,
    content_hash   text,
    source         text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS books_genre_idx  ON books (genre);
CREATE INDEX IF NOT EXISTS books_author_idx ON books (author);
CREATE INDEX IF NOT EXISTS books_tags_idx   ON books USING gin (tags);

-- Free-text search over title/author/description. Complements vector search for
-- exact-name lookups ("do you have Piranesi?"), which embeddings handle poorly.
CREATE INDEX IF NOT EXISTS books_fts_idx ON books
    USING gin (to_tsvector('english',
        coalesce(title, '') || ' ' || coalesce(author, '') || ' ' || coalesce(description, '')));

-- -----------------------------------------------------------------------------
-- book_chunks — embedded content. One row per chunk of a book.
--
-- IMPORTANT — WHY book_id IS NULLABLE AND HAS NO FOREIGN KEY:
-- n8n's "Postgres PGVector Store" insert node writes ONLY these four columns:
--   id (uuid, generated), text, metadata, embedding
-- It does not know about, and never supplies, book_id or chunk_index. If book_id
-- were NOT NULL (or a FK), every insert from the workflow would fail. The book
-- link therefore lives where n8n actually puts it: metadata->>'book_id', set by
-- the document loader in the workflow.
--
-- CONSEQUENCE, and how it is handled:
--   * Re-indexing a book cannot rely on a unique constraint, so the workflow
--     deletes that book's previous chunks (parameterised, by metadata->>'book_id')
--     AFTER the new ones are successfully embedded. See docs/ARCHITECTURE.md §4.
--   * Orphaned chunks (book row deleted, chunks left behind) are possible; the
--     cleanup query at the bottom of this file finds and removes them.
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS book_chunks (
    id          uuid NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    book_id     text,
    chunk_index int,
    text        text,
    -- Denormalised book fields, so search can filter (e.g. by genre) without a join.
    metadata    jsonb NOT NULL DEFAULT '{}'::jsonb,
    -- halfvec, not vector. VERIFIED against pgvector 0.8.7: a plain `vector`
    -- column cannot be HNSW-indexed above 2000 dimensions, and this column is
    -- 3072, so the index creation fails with
    --   "column cannot have more than 2000 dimensions for hnsw index".
    -- halfvec stores 2 bytes per dimension instead of 4 (up to 4000 dims
    -- indexable), halving storage to ~6 KB per chunk. n8n still sends a normal
    -- vector; PostgreSQL casts it on insert. See ADR-002.
    embedding   halfvec(3072),
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS book_chunks_book_id_idx ON book_chunks (book_id);
CREATE INDEX IF NOT EXISTS book_chunks_metadata_idx ON book_chunks USING gin (metadata);

-- Approximate nearest-neighbour index, cosine metric (correct for normalised
-- text embeddings).
--
-- The column is halfvec, so this index is legal at 3072 dimensions (pgvector
-- caps plain `vector` HNSW at 2000). Because the indexed expression is the bare
-- column, `ORDER BY embedding <=> $1` uses the index directly.
--
-- If you ever switch the column back to `vector(3072)`, you MUST replace this
-- with an expression index, which pgvector does allow:
--   CREATE INDEX ... ON book_chunks USING hnsw ((embedding::halfvec(3072)) halfvec_cosine_ops);
-- but then the ORDER BY must cast identically or the index is silently unused
-- and every search degrades to a sequential scan.
--
-- HNSW needs no training pass, so it is correct while ingest is still filling the
-- table (ivfflat would return poor results until it has been trained).
CREATE INDEX IF NOT EXISTS book_chunks_embedding_idx ON book_chunks
    USING hnsw (embedding halfvec_cosine_ops);

-- -----------------------------------------------------------------------------
-- match_book_chunks — the retrieval function behind the agent's primary tool.
--
-- Parameterised by design. The agent NEVER writes SQL: it supplies a query
-- string (embedded by n8n) and optional filters. See docs/DECISIONS.md ADR-005.
--
-- NOTE: book_id is read from metadata->>'book_id', not from the book_id column,
-- because that is where n8n's vector-store insert actually puts it (see the
-- book_chunks comment above). The content column is named "text" for the same
-- reason: it is the column name n8n uses.
--
-- NOTE: n8n's own retrieve-as-tool mode does NOT call this function — it issues
-- its own inline similarity query. This function exists for the SQL-side tools
-- and for anyone querying the library directly.
-- -----------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION match_book_chunks(
    query_embedding halfvec(3072),
    match_count     int  DEFAULT 8,
    filter_genre    text DEFAULT NULL,
    filter_book_id  text DEFAULT NULL
)
RETURNS TABLE (
    book_id    text,
    title      text,
    author     text,
    genre      text,
    chunk_text text,
    similarity double precision
)
LANGUAGE plpgsql
STABLE
AS $$
BEGIN
    RETURN QUERY
    SELECT c.metadata->>'book_id'                        AS book_id,
           coalesce(b.title, c.metadata->>'title')       AS title,
           coalesce(b.author, c.metadata->>'author')     AS author,
           coalesce(b.genre, c.metadata->>'genre')       AS genre,
           c.text                                        AS chunk_text,
           1 - (c.embedding <=> query_embedding)          AS similarity
    FROM book_chunks c
    LEFT JOIN books b ON b.book_id = c.metadata->>'book_id'
    WHERE c.embedding IS NOT NULL
      AND (filter_genre   IS NULL OR coalesce(b.genre, c.metadata->>'genre') = filter_genre)
      AND (filter_book_id IS NULL OR c.metadata->>'book_id' = filter_book_id)
    ORDER BY c.embedding <=> query_embedding
    LIMIT greatest(match_count, 1);
END;
$$;

-- -----------------------------------------------------------------------------
-- get_book — exact metadata lookup by id. Parameterised, read-only.
-- -----------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION get_book(p_book_id text)
RETURNS TABLE (
    book_id        text,
    title          text,
    author         text,
    genre          text,
    tags           text[],
    published_year int,
    page_count     int,
    language       text,
    rating         numeric,
    description    text
)
LANGUAGE sql
STABLE
AS $$
    SELECT b.book_id, b.title, b.author, b.genre, b.tags, b.published_year,
           b.page_count, b.language, b.rating, b.description
    FROM books b
    WHERE b.book_id = p_book_id;
$$;

-- -----------------------------------------------------------------------------
-- list_books — browse/search the catalogue for open-ended questions
-- ("what science fiction do you have?", "anything by Le Guin?").
--
-- WHY ONE PARAMETER: the workflow calls this from an n8n Postgres *tool*, where
-- values come from the model via $fromAI(). A single argument avoids depending on
-- the order in which a model supplies several arguments, which is not guaranteed.
-- The parameter is matched loosely across title, author, genre, tags and
-- description, so one field covers genre/author/title browsing.
-- -----------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION list_books(
    search_query text DEFAULT NULL,
    limit_n      int  DEFAULT 25,
    offset_n     int  DEFAULT 0
)
RETURNS TABLE (
    book_id        text,
    title          text,
    author         text,
    genre          text,
    published_year int,
    rating         numeric,
    description    text
)
LANGUAGE sql
STABLE
AS $$
    SELECT b.book_id, b.title, b.author, b.genre, b.published_year,
           b.rating, b.description
    FROM books b
    WHERE search_query IS NULL
       OR search_query = ''
       OR b.title       ILIKE '%' || search_query || '%'
       OR b.author      ILIKE '%' || search_query || '%'
       OR b.genre       ILIKE '%' || search_query || '%'
       OR b.description ILIKE '%' || search_query || '%'
       OR EXISTS (SELECT 1 FROM unnest(b.tags) AS t WHERE t ILIKE '%' || search_query || '%')
    ORDER BY b.rating DESC NULLS LAST, b.title
    LIMIT least(greatest(limit_n, 1), 100)
    OFFSET greatest(offset_n, 0);
$$;

-- -----------------------------------------------------------------------------
-- delete_book — remove a book AND its chunks atomically.
--
-- Needed because book_chunks has no foreign key (n8n's vector store does not
-- supply book_id, see the book_chunks comment). Deleting from `books` alone
-- would leave searchable orphan chunks behind, so provide one correct path.
-- -----------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION delete_book(p_book_id text)
RETURNS TABLE (deleted_chunks bigint, deleted_book boolean)
LANGUAGE plpgsql
AS $$
DECLARE
    chunk_count  bigint;
    book_deleted boolean;
BEGIN
    DELETE FROM book_chunks WHERE metadata->>'book_id' = p_book_id;
    GET DIAGNOSTICS chunk_count = ROW_COUNT;

    DELETE FROM books WHERE book_id = p_book_id;
    GET DIAGNOSTICS book_deleted = ROW_COUNT;

    RETURN QUERY SELECT chunk_count, (book_deleted OR chunk_count > 0);
END;
$$;

-- -----------------------------------------------------------------------------
-- Convenience: keep books.updated_at honest without trigger boilerplate.
-- -----------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION touch_books_updated_at()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS books_touch_updated_at ON books;
CREATE TRIGGER books_touch_updated_at
    BEFORE UPDATE ON books
    FOR EACH ROW
    EXECUTE FUNCTION touch_books_updated_at();

-- -----------------------------------------------------------------------------
-- Least-privilege role for the agent's read-only tools (ADR-005 defence in depth).
-- Wrapped in a DO block so re-running does not error when the role exists.
-- The password is intentionally NOT set here: grant one interactively if you
-- choose to actually use this role, and put it in the n8n credential, not in SQL.
-- -----------------------------------------------------------------------------
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'bookrag_readonly') THEN
        CREATE ROLE bookrag_readonly NOLOGIN;
    END IF;
END
$$;

GRANT USAGE ON SCHEMA public TO bookrag_readonly;
GRANT SELECT ON books, book_chunks TO bookrag_readonly;
GRANT EXECUTE ON FUNCTION match_book_chunks(halfvec, int, text, text) TO bookrag_readonly;
GRANT EXECUTE ON FUNCTION get_book(text) TO bookrag_readonly;
GRANT EXECUTE ON FUNCTION list_books(text, int, int) TO bookrag_readonly;

-- Note: delete_book is deliberately NOT granted to bookrag_readonly. The agent's
-- read-only role must not be able to remove library content.

-- -----------------------------------------------------------------------------
-- Maintenance: find (and optionally remove) chunks whose book no longer exists.
--
-- Because n8n writes the book link into metadata->>'book_id' rather than a real
-- foreign key, deleting a row from `books` does NOT remove its chunks. Run the
-- SELECT first, then the DELETE if the result looks right.
-- -----------------------------------------------------------------------------

-- Step 1 — inspect:
--   SELECT count(*) AS orphan_chunks
--   FROM book_chunks c
--   WHERE c.metadata->>'book_id' IS NOT NULL
--     AND NOT EXISTS (SELECT 1 FROM books b WHERE b.book_id = c.metadata->>'book_id');

-- Step 2 — remove:
--   DELETE FROM book_chunks c
--   WHERE c.metadata->>'book_id' IS NOT NULL
--     AND NOT EXISTS (SELECT 1 FROM books b WHERE b.book_id = c.metadata->>'book_id');

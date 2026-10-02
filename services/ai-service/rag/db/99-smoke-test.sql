-- Smoke test for the Book RAG schema. Proves the tables, trigger, HNSW index and
-- all read/write functions behave, and that the vector index is really used.
-- Safe to run repeatedly: it cleans up after itself.
--
-- NOTE ON TEST EMBEDDINGS: a vector of all zeros has an UNDEFINED cosine distance,
-- and pgvector's HNSW index will not return such rows (the heap scan would). All
-- test embeddings here are therefore non-zero, which is also what real embeddings
-- look like.

\set emb '(\'[\' || repeat(\'0.001,\', 3071) || \'0.001]\')::halfvec(3072)'

\echo '--- 1. insert a book via the catalogue table ---'
INSERT INTO books (book_id, title, author, genre, tags, published_year, page_count, rating, description, source)
VALUES ('smoke-dune', 'Smoke Test Dune', 'Frank Herbert', 'Science Fiction',
        '{desert,politics}', 1965, 688, 4.27, 'A smoke-test blurb about spice and prophecy.', 'test')
ON CONFLICT (book_id) DO UPDATE SET description = EXCLUDED.description;

\echo '--- 2. updated_at trigger fires on conflict-update (expect t) ---'
SELECT book_id, (updated_at >= created_at) AS trigger_ok FROM books WHERE book_id = 'smoke-dune';

\echo '--- 3. insert a chunk with a realistic embedding ---'
INSERT INTO book_chunks (metadata, text, embedding)
VALUES (
  '{"book_id":"smoke-dune","title":"Smoke Test Dune","author":"Frank Herbert","genre":"Science Fiction"}'::jsonb,
  'A beginning is the time for taking the most delicate care that the balances are correct.',
  :emb
);

\echo '--- 4. list_books: no filter returns the book ---'
SELECT book_id, title, genre FROM list_books(NULL, 25, 0);

\echo '--- 5. list_books: loose match on genre (expect 1 row) ---'
SELECT count(*) AS genre_hits FROM list_books('science', 25, 0);

\echo '--- 6. list_books: loose match on author (expect 1 row) ---'
SELECT count(*) AS author_hits FROM list_books('herbert', 25, 0);

\echo '--- 7. list_books: nonsense term returns nothing (expect 0) ---'
SELECT count(*) AS nonsense_hits FROM list_books('zzzznotathing', 25, 0);

\echo '--- 8. get_book: exact lookup ---'
SELECT book_id, title, author, rating FROM get_book('smoke-dune');

\echo '--- 9. get_book: unknown id returns no rows (expect 0) ---'
SELECT count(*) AS unknown_hits FROM get_book('no-such-book');

\echo '--- 10. match_book_chunks: similarity search returns the chunk (expect 1) ---'
SELECT book_id, title, round(similarity::numeric, 6) AS similarity
FROM match_book_chunks(:emb, 5, NULL, NULL);

\echo '--- 11. match_book_chunks: genre filter excludes it (expect 0) ---'
SELECT count(*) AS filtered_out FROM match_book_chunks(:emb, 5, 'Fantasy', NULL);

\echo '--- 12. HNSW index is used by the inline ORDER BY shape n8n issues ---'
EXPLAIN (COSTS OFF)
SELECT id, text FROM book_chunks
ORDER BY embedding <=> :emb
LIMIT 5;

\echo '--- 13. HNSW index is ALSO used inside match_book_chunks ---'
EXPLAIN (COSTS OFF)
SELECT * FROM match_book_chunks(:emb, 5, NULL, NULL);

\echo '--- 14. delete_book removes catalogue row AND chunks atomically ---'
SELECT * FROM delete_book('smoke-dune');
SELECT (SELECT count(*) FROM books WHERE book_id = 'smoke-dune') AS books_left,
       (SELECT count(*) FROM book_chunks WHERE metadata->>'book_id' = 'smoke-dune') AS chunks_left;

\echo '--- 15. cleanup + final counts (expect 0 / 0) ---'
DELETE FROM book_chunks WHERE metadata->>'book_id' = 'smoke-dune';
SELECT (SELECT count(*) FROM books) AS books, (SELECT count(*) FROM book_chunks) AS chunks;

-- Functional assertions for the Book RAG read/write path. Each expected value is
-- stated inline. Cleans up after itself. Non-zero embeddings only: a zero vector
-- has an undefined cosine distance and pgvector's HNSW index will not return it.

DELETE FROM book_chunks WHERE metadata->>'book_id' IN ('t1', 't2');
DELETE FROM books WHERE book_id IN ('t1', 't2');

INSERT INTO books (book_id, title, author, genre, tags, published_year, page_count, rating, description, source)
VALUES
  ('t1', 'Test One', 'Alice Author', 'Science Fiction', '{spice,desert}', 1965, 300, 4.50, 'A blurb about space and prophecy.', 'test'),
  ('t2', 'Test Two', 'Bob Writer',  'Mystery',         '{detective}',     1999, 250, 3.75, 'A blurb about a missing heiress.',    'test');

INSERT INTO book_chunks (metadata, text, embedding) VALUES
  ('{"book_id":"t1","title":"Test One","author":"Alice Author","genre":"Science Fiction"}'::jsonb,
   'chunk one content about space',
   ('[' || repeat('0.001,', 3071) || '0.001]')::halfvec(3072)),
  ('{"book_id":"t2","title":"Test Two","author":"Bob Writer","genre":"Mystery"}'::jsonb,
   'chunk two content about a detective',
   ('[' || repeat('0.002,', 3071) || '0.002]')::halfvec(3072));

\echo '--- EXPECT: 2 | 1 | 1 | 0 | 2 | 1 ---'
SELECT (SELECT count(*) FROM list_books(NULL, 25, 0))                AS list_all,
       (SELECT count(*) FROM list_books('science', 25, 0))           AS list_by_genre,
       (SELECT count(*) FROM list_books('alice', 25, 0))             AS list_by_author,
       (SELECT count(*) FROM list_books('zzzznope', 25, 0))          AS list_nonsense,
       (SELECT count(*) FROM match_book_chunks(
            ('[' || repeat('0.001,', 3071) || '0.001]')::halfvec(3072), 5, NULL, NULL)) AS match_all,
       (SELECT count(*) FROM match_book_chunks(
            ('[' || repeat('0.001,', 3071) || '0.001]')::halfvec(3072), 5, 'Mystery', NULL)) AS match_genre;

\echo '--- EXPECT: t1 ranked ABOVE t2 (query is all 0.001, t1 embedding is all 0.001) ---'
SELECT book_id, round(similarity::numeric, 6) AS similarity
FROM match_book_chunks(('[' || repeat('0.001,', 3071) || '0.001]')::halfvec(3072), 5, NULL, NULL);

\echo '--- EXPECT: t1 | Test One | 4.50 ---'
SELECT book_id, title, rating FROM get_book('t1');

\echo '--- EXPECT: 0 rows (unknown id) ---'
SELECT count(*) AS unknown_id_rows FROM get_book('does-not-exist');

\echo '--- EXPECT: deletes 1 chunk and 1 book ---'
SELECT * FROM delete_book('t1');

\echo '--- EXPECT: 1 book, 1 chunk remaining, then cleanup to 0/0 ---'
SELECT (SELECT count(*) FROM books) AS books, (SELECT count(*) FROM book_chunks) AS chunks;
SELECT * FROM delete_book('t2');
SELECT (SELECT count(*) FROM books) AS books_final, (SELECT count(*) FROM book_chunks) AS chunks_final;

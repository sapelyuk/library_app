-- 001_init.sql — initial schema of the Book Service database.
-- Applied by the migration tool once the PostgreSQL repository is added;
-- the service currently runs on the in-memory storage.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE books (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    isbn           VARCHAR(13)  NOT NULL,
    title          TEXT         NOT NULL,
    author         TEXT         NOT NULL,
    publisher      TEXT         NOT NULL DEFAULT '',
    published_year SMALLINT     NOT NULL CHECK (published_year BETWEEN 1445 AND 2300),
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT books_isbn_key UNIQUE (isbn)
);

CREATE INDEX books_title_idx ON books (title);
CREATE INDEX books_author_idx ON books (author);

CREATE TABLE book_copies (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    book_id    UUID        NOT NULL REFERENCES books (id) ON DELETE CASCADE,
    barcode    VARCHAR(64) NOT NULL,
    status     VARCHAR(16) NOT NULL DEFAULT 'AVAILABLE'
               CHECK (status IN ('AVAILABLE', 'ON_LOAN', 'LOST', 'MAINTENANCE')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT book_copies_barcode_key UNIQUE (barcode)
);

-- Supports "give me the first available copy of this book" lookups.
CREATE INDEX book_copies_book_status_idx ON book_copies (book_id, status, created_at);

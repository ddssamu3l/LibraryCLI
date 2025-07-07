# Model Comparison Project

**Project Date:** 2025-07-02 20:11:00

This project contains model comparison results.

- `preedit` branch: Original baseline codebase
- `beetle` branch: Beetle model's response
- `sonnet` branch: Sonnet model's response
- `rewrite` branch: Rewritten codebase

## Library Management CLI (SQLite Rewrite)

This branch stores **all data**—books, members, checkout history, and full-text search index—in a single on-disk SQLite database (`library.db`).  The design scales to millions of books with megabytes of text each by using:

* FTS5 virtual table (`books_fts`) for instant full-text search of title, author, and content.
* A separate `checkouts` table to keep historical records while the `books` row tracks current availability.
* WAL mode + prepared statements + transactions for high-throughput batch inserts.

## Requirements

1. Go 1.20+
2. CGO enabled (default on macOS/Linux).  On Windows make sure you have gcc installed (e.g. via **mingw-w64**).
3. Build tag `sqlite_fts5` to compile the native driver with FTS5 support.

```bash
# install deps
go mod tidy

# run the CLI
go run -tags sqlite_fts5 .

# run tests
go test ./... -tags sqlite_fts5
```

## CLI Usage

| Command          | Description |
|------------------|-------------|
| `add book`       | Prompts for title, author, then optional file path to stream full text. |
| `add member`     | Registers a library member. |
| `update content` | Attach or replace the book's text later by providing book ID and file path. |
| `list books`     | Lists all books without loading their heavy text columns. |
| `list members`   | Lists members. |
| `search book`    | Full-text search across title, author, and book text. |
| `checkout`       | Checkout a book (updates `books`, inserts into `checkouts`). |
| `return`         | Return a book (records `return_time` in `checkouts`). |
| `exit`           | Quit. |

## Internals

```
books(id PK, title, author, content, available, borrower_id FK)  <-- current state
members(id PK, name)
checkouts(id PK, book_id FK, member_id FK, checkout_time, return_time)
books_fts(title, author, content)             <-- FTS5 virtual table
```

Triggers keep `books_fts` in sync with the `books` table, and WAL mode improves concurrency. Large-scale bulk imports should wrap many `AddBookFromFile` calls in one transaction; the driver, prepared statement, and buffered file streaming keep memory usage bounded (only one book's text in RAM at a time).

Happy reading! 😄

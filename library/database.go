package library

import (
	"bufio"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Database provides high-level helpers around a SQLite connection.
type Database struct {
	db *sql.DB

	addBookStmt   *sql.Stmt
	addMemberStmt *sql.Stmt
}

// NewDatabase opens (or creates) the SQLite database at dbPath, applies schema
// migrations, and prepares common statements.
func NewDatabase(dbPath string) (*Database, error) {
	// Ensure directory exists so first-run succeeds.
	if dir := filepath.Dir(dbPath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	// Enable busy_timeout and foreign keys.
	dsn := fmt.Sprintf("file:%s?_busy_timeout=5000&_foreign_keys=1", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := applyMigrations(db); err != nil {
		db.Close()
		return nil, err
	}

	database := &Database{db: db}
	if err := database.prepareStatements(); err != nil {
		db.Close()
		return nil, err
	}
	return database, nil
}

// Close releases prepared statements and closes the DB.
func (d *Database) Close() error {
	if d.addBookStmt != nil {
		d.addBookStmt.Close()
	}
	if d.addMemberStmt != nil {
		d.addMemberStmt.Close()
	}
	return d.db.Close()
}

// ---------------------------------------------------------------------------
// Schema migration
// ---------------------------------------------------------------------------

const schemaVersion = 2

func applyMigrations(db *sql.DB) error {
	// WAL improves write concurrency.
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return fmt.Errorf("enable WAL: %w", err)
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT);`); err != nil {
		return err
	}

	var current int
	_ = db.QueryRow(`SELECT value FROM meta WHERE key='schema_version';`).Scan(&current)
	if current >= schemaVersion {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS members (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL
        );`,
		`CREATE TABLE IF NOT EXISTS books (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            title TEXT NOT NULL,
            author TEXT NOT NULL,
            content TEXT NOT NULL,
            available BOOLEAN NOT NULL DEFAULT 1,
            borrower_id INTEGER REFERENCES members(id)
        );`,
		`CREATE TABLE IF NOT EXISTS checkouts (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            book_id INTEGER NOT NULL REFERENCES books(id),
            member_id INTEGER NOT NULL REFERENCES members(id),
            checkout_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
            return_time DATETIME
        );`,
		`CREATE TABLE IF NOT EXISTS reservations (
		    id INTEGER PRIMARY KEY AUTOINCREMENT,
		    book_id INTEGER NOT NULL REFERENCES books(id),
		    member_id INTEGER NOT NULL REFERENCES members(id),
		    reservation_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		    fulfilled_time DATETIME,
		    UNIQUE(book_id, member_id)
		);`,
		// FTS5 virtual table
		`CREATE VIRTUAL TABLE IF NOT EXISTS books_fts USING fts5(
            title, author, content, content='books', content_rowid='id'
        );`,
		// Triggers to keep FTS in sync
		`CREATE TRIGGER IF NOT EXISTS trg_books_ai AFTER INSERT ON books BEGIN
            INSERT INTO books_fts(rowid,title,author,content) VALUES(new.id,new.title,new.author,new.content);
        END;`,
		`CREATE TRIGGER IF NOT EXISTS trg_books_ad AFTER DELETE ON books BEGIN
            INSERT INTO books_fts(books_fts, rowid, title, author, content) VALUES('delete',old.id,old.title,old.author,old.content);
        END;`,
		`CREATE TRIGGER IF NOT EXISTS trg_books_au AFTER UPDATE ON books BEGIN
            INSERT INTO books_fts(books_fts, rowid, title, author, content) VALUES('delete',old.id,old.title,old.author,old.content);
            INSERT INTO books_fts(rowid,title,author,content) VALUES(new.id,new.title,new.author,new.content);
        END;`,
		`INSERT INTO meta(key,value) VALUES('schema_version',?)
            ON CONFLICT(key) DO UPDATE SET value=excluded.value;`,
	}

	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt, schemaVersion); err != nil {
			return fmt.Errorf("apply migration: %w", err)
		}
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Prepared statements
// ---------------------------------------------------------------------------

func (d *Database) prepareStatements() error {
	var err error
	if d.addBookStmt, err = d.db.Prepare(`INSERT INTO books(title,author,content) VALUES(?,?,?)`); err != nil {
		return err
	}
	if d.addMemberStmt, err = d.db.Prepare(`INSERT INTO members(name) VALUES(?)`); err != nil {
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// CRUD helpers
// ---------------------------------------------------------------------------

func (d *Database) AddMember(name string) (int64, error) {
	res, err := d.addMemberStmt.Exec(name)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// AddBook inserts a book when you already have the full content in memory.
func (d *Database) AddBook(title, author, content string) (int64, error) {
	res, err := d.addBookStmt.Exec(title, author, content)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// AddBookFromReader streams the content from r and avoids holding more than
// one book's text in memory at a time.
func (d *Database) AddBookFromReader(title, author string, r io.Reader) (int64, error) {
	var sb strings.Builder
	br := bufio.NewReader(r)
	if _, err := br.WriteTo(&sb); err != nil {
		return 0, err
	}
	return d.AddBook(title, author, sb.String())
}

func (d *Database) GetBook(id int64) (*Book, error) {
	var b Book
	err := d.db.QueryRow(`SELECT id,title,author,content,available,COALESCE(borrower_id,0) FROM books WHERE id=?`, id).
		Scan(&b.ID, &b.Title, &b.Author, &b.Content, &b.Available, &b.BorrowerID)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// GetAllBooks returns metadata only (no heavy content) for quick listing.
func (d *Database) GetAllBooks() ([]*Book, error) {
	rows, err := d.db.Query(`SELECT id,title,author,available,COALESCE(borrower_id,0) FROM books ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []*Book
	for rows.Next() {
		var b Book
		if err := rows.Scan(&b.ID, &b.Title, &b.Author, &b.Available, &b.BorrowerID); err != nil {
			return nil, err
		}
		books = append(books, &b)
	}
	return books, nil
}

// SearchBooks leverages FTS5. It returns lightweight rows.
func (d *Database) SearchBooks(q string) ([]*Book, error) {
	if strings.TrimSpace(q) == "" {
		return []*Book{}, nil
	}
	rows, err := d.db.Query(`
        SELECT b.id, b.title, b.author, b.available, COALESCE(b.borrower_id,0)
        FROM books_fts fts
        JOIN books b ON b.id = fts.rowid
        WHERE books_fts MATCH ?
        ORDER BY rank;`, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Book
	for rows.Next() {
		var b Book
		if err := rows.Scan(&b.ID, &b.Title, &b.Author, &b.Available, &b.BorrowerID); err != nil {
			return nil, err
		}
		results = append(results, &b)
	}
	return results, nil
}

// CheckoutBook records the checkout and updates availability in one transaction.
func (d *Database) CheckoutBook(bookID, memberID int64) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var avail bool
	if err := tx.QueryRow(`SELECT available FROM books WHERE id=?`, bookID).Scan(&avail); err != nil {
		return err
	}
	if !avail {
		return fmt.Errorf("book %d already checked out", bookID)
	}

	if _, err := tx.Exec(`INSERT INTO checkouts(book_id,member_id) VALUES(?,?)`, bookID, memberID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE books SET available=0, borrower_id=? WHERE id=?`, memberID, bookID); err != nil {
		return err
	}
	return tx.Commit()
}

// ReserveBook places a reservation for a book by a member, or checks it out immediately if available.
//
// The function performs comprehensive validation:
// - Verifies both book and member exist in the database
// - Prevents duplicate reservations (member already has an active reservation for this book)
// - Prevents reserving a book the member already has checked out
//
// Behavior depends on book availability:
// - If book is available: immediately checks it out to the member
// - If book is unavailable: adds the member to the reservation queue (FIFO order)
//
// All operations are performed within a single database transaction to ensure consistency
// and prevent race conditions in concurrent scenarios.
func (d *Database) ReserveBook(bookID, memberID int64) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists bool
	// Verify book exists
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM books WHERE id=?)`, bookID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("book %d does not exist", bookID)
	}

	// Verify member exists
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM members WHERE id=?)`, memberID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("member %d does not exist", memberID)
	}

	// Check duplicate active reservation
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM reservations WHERE book_id=? AND member_id=? AND fulfilled_time IS NULL)`, bookID, memberID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("you already have a reservation for this book")
	}

	// Check if member already has this book checked out
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM checkouts WHERE book_id=? AND member_id=? AND return_time IS NULL)`, bookID, memberID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("you can't reserve this book because you have already checked it out")
	}

	// Check availability
	var avail bool
	if err := tx.QueryRow(`SELECT available FROM books WHERE id=?`, bookID).Scan(&avail); err != nil {
		return err
	}

	if avail {
		// Immediate checkout
		if _, err := tx.Exec(`INSERT INTO checkouts(book_id,member_id) VALUES(?,?)`, bookID, memberID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE books SET available=0, borrower_id=? WHERE id=?`, memberID, bookID); err != nil {
			return err
		}
	} else {
		// Queue reservation
		if _, err := tx.Exec(`INSERT INTO reservations(book_id,member_id) VALUES(?,?)`, bookID, memberID); err != nil {
			// Handle database constraint violations with user-friendly messages
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				return fmt.Errorf("you already have a reservation for this book")
			}
			return err
		}
	}

	return tx.Commit()
}

// ReturnBook marks a book as returned and automatically assigns it to the next member in the reservation queue.
//
// The function performs the following operations within a single transaction:
// 1. Validates that the book is currently checked out
// 2. Updates the checkout record with a return timestamp
// 3. Checks for active reservations for this book (ordered by reservation_time)
// 4. If reservations exist:
//   - Automatically checks out the book to the next member in queue (FIFO)
//   - Marks their reservation as fulfilled
//   - Book remains unavailable but with new borrower
//
// 5. If no reservations exist:
//   - Makes the book available for general checkout
//
// Returns the ID of the member who returned the book.
// All operations are atomic to prevent race conditions during concurrent returns and reservations.
func (d *Database) ReturnBook(bookID int64) (int64, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var chkID, memberID int64
	err = tx.QueryRow(`SELECT id, member_id FROM checkouts WHERE book_id=? AND return_time IS NULL`, bookID).
		Scan(&chkID, &memberID)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("book %d is not checked out", bookID)
	}
	if err != nil {
		return 0, err
	}

	// Mark as returned
	if _, err := tx.Exec(`UPDATE checkouts SET return_time=? WHERE id=?`, time.Now(), chkID); err != nil {
		return 0, err
	}

	// Fetch next reservation (oldest)
	var nextMemberID sql.NullInt64
	var reservationID sql.NullInt64
	err = tx.QueryRow(`SELECT id, member_id FROM reservations WHERE book_id=? AND fulfilled_time IS NULL ORDER BY reservation_time ASC LIMIT 1`, bookID).
		Scan(&reservationID, &nextMemberID)

	if err == nil && nextMemberID.Valid {
		// Assign to next member
		if _, err := tx.Exec(`INSERT INTO checkouts(book_id,member_id) VALUES(?,?)`, bookID, nextMemberID.Int64); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`UPDATE books SET borrower_id=? WHERE id=?`, nextMemberID.Int64, bookID); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`UPDATE reservations SET fulfilled_time=? WHERE id=?`, time.Now(), reservationID.Int64); err != nil {
			return 0, err
		}
	} else if err == sql.ErrNoRows {
		// No reservations, make available
		if _, err := tx.Exec(`UPDATE books SET available=1, borrower_id=NULL WHERE id=?`, bookID); err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	}

	return memberID, tx.Commit()
}

// UpdateBookContent replaces the book's stored text.
func (d *Database) UpdateBookContent(bookID int64, content string) error {
	_, err := d.db.Exec(`UPDATE books SET content=? WHERE id=?`, content, bookID)
	return err
}

// GetMember fetches a single member.
func (d *Database) GetMember(id int64) (*Member, error) {
	var m Member
	if err := d.db.QueryRow(`SELECT id,name FROM members WHERE id=?`, id).Scan(&m.ID, &m.Name); err != nil {
		return nil, err
	}
	return &m, nil
}

// GetAllMembers returns all members.
func (d *Database) GetAllMembers() ([]*Member, error) {
	rows, err := d.db.Query(`SELECT id,name FROM members ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []*Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.Name); err != nil {
			return nil, err
		}
		members = append(members, &m)
	}
	return members, nil
}

// GetReservations returns active reservations for a book ordered by time.
func (d *Database) GetReservations(bookID int64) ([]*Member, error) {
	rows, err := d.db.Query(`SELECT m.id, m.name FROM reservations r JOIN members m ON m.id = r.member_id WHERE r.book_id = ? AND r.fulfilled_time IS NULL ORDER BY r.reservation_time ASC`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []*Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.Name); err != nil {
			return nil, err
		}
		members = append(members, &m)
	}
	return members, nil
}

// GetMemberReservations returns active reservations for a member.
func (d *Database) GetMemberReservations(memberID int64) ([]*Book, error) {
	rows, err := d.db.Query(`SELECT b.id, b.title, b.author, b.available, COALESCE(b.borrower_id,0) FROM reservations r JOIN books b ON b.id = r.book_id WHERE r.member_id = ? AND r.fulfilled_time IS NULL ORDER BY r.reservation_time ASC`, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []*Book
	for rows.Next() {
		var b Book
		if err := rows.Scan(&b.ID, &b.Title, &b.Author, &b.Available, &b.BorrowerID); err != nil {
			return nil, err
		}
		books = append(books, &b)
	}
	return books, nil
}

// CancelReservation deletes an active reservation.
func (d *Database) CancelReservation(bookID, memberID int64) error {
	result, err := d.db.Exec(`DELETE FROM reservations WHERE book_id=? AND member_id=? AND fulfilled_time IS NULL`, bookID, memberID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("no active reservation found for member %d on book %d", memberID, bookID)
	}
	return nil
}

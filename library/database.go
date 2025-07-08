package library

import (
	"bufio"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
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

const schemaVersion = 3

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
            name TEXT NOT NULL,
            password_hash TEXT NOT NULL DEFAULT ''
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
		// Migration for existing members without passwords
		`UPDATE members SET password_hash = '' WHERE password_hash IS NULL;`,
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
	if d.addMemberStmt, err = d.db.Prepare(`INSERT INTO members(name,password_hash) VALUES(?,?)`); err != nil {
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// Authentication helpers
// ---------------------------------------------------------------------------

// HashPassword creates a bcrypt hash of the password
func (d *Database) HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword verifies a password against its hash
func (d *Database) CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// AuthenticateMember verifies member credentials
func (d *Database) AuthenticateMember(memberID int64, password string) error {
	var hash string
	err := d.db.QueryRow(`SELECT password_hash FROM members WHERE id=?`, memberID).Scan(&hash)
	if err == sql.ErrNoRows {
		return fmt.Errorf("member not found")
	}
	if err != nil {
		return err
	}

	if hash == "" {
		return fmt.Errorf("member has no password set - please contact administrator")
	}

	if !d.CheckPassword(password, hash) {
		return fmt.Errorf("invalid password")
	}

	return nil
}

// ResetMemberPassword updates a member's password
func (d *Database) ResetMemberPassword(memberID int64, newPassword string) error {
	hash, err := d.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	result, err := d.db.Exec(`UPDATE members SET password_hash=? WHERE id=?`, hash, memberID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("member not found")
	}

	return nil
}

// ---------------------------------------------------------------------------
// CRUD helpers (updated)
// ---------------------------------------------------------------------------

func (d *Database) AddMember(name, password string) (int64, error) {
	hash, err := d.HashPassword(password)
	if err != nil {
		return 0, fmt.Errorf("failed to hash password: %w", err)
	}

	res, err := d.addMemberStmt.Exec(name, hash)
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

func (d *Database) GetAllBooks() ([]*Book, error) {
	rows, err := d.db.Query(`SELECT id,title,author,content,available,COALESCE(borrower_id,0) FROM books ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []*Book
	for rows.Next() {
		var b Book
		if err := rows.Scan(&b.ID, &b.Title, &b.Author, &b.Content, &b.Available, &b.BorrowerID); err != nil {
			return nil, err
		}
		books = append(books, &b)
	}
	return books, rows.Err()
}

func (d *Database) SearchBooks(q string) ([]*Book, error) {
	// Use FTS5 for search
	query := `SELECT b.id, b.title, b.author, b.content, b.available, COALESCE(b.borrower_id,0)
              FROM books_fts fts
              JOIN books b ON fts.rowid = b.id
              WHERE books_fts MATCH ?
              ORDER BY rank`

	rows, err := d.db.Query(query, q)
	if err != nil {
		// If FTS fails, fall back to LIKE search
		fallbackQuery := `SELECT id,title,author,content,available,COALESCE(borrower_id,0) 
                          FROM books 
                          WHERE title LIKE ? OR author LIKE ? 
                          ORDER BY id`
		likePattern := "%" + q + "%"
		rows, err = d.db.Query(fallbackQuery, likePattern, likePattern)
		if err != nil {
			return nil, err
		}
	}
	defer rows.Close()

	var books []*Book
	for rows.Next() {
		var b Book
		if err := rows.Scan(&b.ID, &b.Title, &b.Author, &b.Content, &b.Available, &b.BorrowerID); err != nil {
			return nil, err
		}
		books = append(books, &b)
	}
	return books, rows.Err()
}

func (d *Database) CheckoutBook(bookID, memberID int64) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Check if book exists and is available
	var available bool
	err = tx.QueryRow(`SELECT available FROM books WHERE id=?`, bookID).Scan(&available)
	if err == sql.ErrNoRows {
		return fmt.Errorf("book not found")
	}
	if err != nil {
		return err
	}
	if !available {
		return fmt.Errorf("book is not available")
	}

	// Update book as checked out
	if _, err := tx.Exec(`UPDATE books SET available=0, borrower_id=? WHERE id=?`, memberID, bookID); err != nil {
		return err
	}

	// Record checkout
	if _, err := tx.Exec(`INSERT INTO checkouts(book_id, member_id) VALUES(?,?)`, bookID, memberID); err != nil {
		return err
	}

	return tx.Commit()
}

func (d *Database) ReserveBook(bookID, memberID int64) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Check if book exists
	var available bool
	var borrowerID sql.NullInt64
	err = tx.QueryRow(`SELECT available, borrower_id FROM books WHERE id=?`, bookID).Scan(&available, &borrowerID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("book not found")
	}
	if err != nil {
		return err
	}

	// If book is available, check it out immediately instead of reserving
	if available {
		// Update book as checked out
		if _, err := tx.Exec(`UPDATE books SET available=0, borrower_id=? WHERE id=?`, memberID, bookID); err != nil {
			return err
		}

		// Record checkout
		if _, err := tx.Exec(`INSERT INTO checkouts(book_id, member_id) VALUES(?,?)`, bookID, memberID); err != nil {
			return err
		}

		return tx.Commit()
	}

	// Book is not available, make a reservation
	// Check if member already has a reservation for this book
	var existingID int64
	err = tx.QueryRow(`SELECT id FROM reservations WHERE book_id=? AND member_id=? AND fulfilled_time IS NULL`, bookID, memberID).Scan(&existingID)
	if err == nil {
		return fmt.Errorf("member already has a reservation for this book")
	}
	if err != sql.ErrNoRows {
		return err
	}

	// Create reservation
	if _, err := tx.Exec(`INSERT INTO reservations(book_id, member_id) VALUES(?,?)`, bookID, memberID); err != nil {
		return err
	}

	return tx.Commit()
}

// ReturnBook marks a book as returned and assigns it to the next person in the reservation queue.
// Returns (returnedByMemberID, assignedToMemberID, error).
// If no one is waiting, assignedToMemberID will be 0.
func (d *Database) ReturnBook(bookID int64) (int64, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Get current borrower
	var borrowerID int64
	var available bool
	err = tx.QueryRow(`SELECT borrower_id, available FROM books WHERE id=?`, bookID).Scan(&borrowerID, &available)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("book not found")
	}
	if err != nil {
		return 0, err
	}
	if available {
		return 0, fmt.Errorf("book is not checked out")
	}

	// Mark current checkout as returned
	if _, err := tx.Exec(`UPDATE checkouts SET return_time=CURRENT_TIMESTAMP WHERE book_id=? AND member_id=? AND return_time IS NULL`, bookID, borrowerID); err != nil {
		return 0, err
	}

	// Check for reservations
	var nextMemberID sql.NullInt64
	err = tx.QueryRow(`SELECT member_id FROM reservations WHERE book_id=? AND fulfilled_time IS NULL ORDER BY reservation_time LIMIT 1`, bookID).Scan(&nextMemberID)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}

	if nextMemberID.Valid {
		// Assign to next member in queue
		if _, err := tx.Exec(`UPDATE books SET borrower_id=? WHERE id=?`, nextMemberID.Int64, bookID); err != nil {
			return 0, err
		}

		// Mark reservation as fulfilled
		if _, err := tx.Exec(`UPDATE reservations SET fulfilled_time=CURRENT_TIMESTAMP WHERE book_id=? AND member_id=?`, bookID, nextMemberID.Int64); err != nil {
			return 0, err
		}

		// Create new checkout record
		if _, err := tx.Exec(`INSERT INTO checkouts(book_id, member_id) VALUES(?,?)`, bookID, nextMemberID.Int64); err != nil {
			return 0, err
		}
	} else {
		// No one waiting, make available
		if _, err := tx.Exec(`UPDATE books SET available=1, borrower_id=NULL WHERE id=?`, bookID); err != nil {
			return 0, err
		}
	}

	return borrowerID, tx.Commit()
}

// ReturnBookWithDetails provides more information about the return process
func (d *Database) ReturnBookWithDetails(bookID int64) (returnedBy int64, assignedTo int64, err error) {
	tx, err := d.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	// Get current borrower
	var borrowerID int64
	var available bool
	err = tx.QueryRow(`SELECT borrower_id, available FROM books WHERE id=?`, bookID).Scan(&borrowerID, &available)
	if err == sql.ErrNoRows {
		return 0, 0, fmt.Errorf("book not found")
	}
	if err != nil {
		return 0, 0, err
	}
	if available {
		return 0, 0, fmt.Errorf("book is not checked out")
	}

	// Mark current checkout as returned
	if _, err := tx.Exec(`UPDATE checkouts SET return_time=CURRENT_TIMESTAMP WHERE book_id=? AND member_id=? AND return_time IS NULL`, bookID, borrowerID); err != nil {
		return 0, 0, err
	}

	// Check for reservations
	var nextMemberID sql.NullInt64
	err = tx.QueryRow(`SELECT member_id FROM reservations WHERE book_id=? AND fulfilled_time IS NULL ORDER BY reservation_time LIMIT 1`, bookID).Scan(&nextMemberID)
	if err != nil && err != sql.ErrNoRows {
		return 0, 0, err
	}

	if nextMemberID.Valid {
		// Assign to next member in queue
		if _, err := tx.Exec(`UPDATE books SET borrower_id=? WHERE id=?`, nextMemberID.Int64, bookID); err != nil {
			return 0, 0, err
		}

		// Mark reservation as fulfilled
		if _, err := tx.Exec(`UPDATE reservations SET fulfilled_time=CURRENT_TIMESTAMP WHERE book_id=? AND member_id=?`, bookID, nextMemberID.Int64); err != nil {
			return 0, 0, err
		}

		// Create new checkout record
		if _, err := tx.Exec(`INSERT INTO checkouts(book_id, member_id) VALUES(?,?)`, bookID, nextMemberID.Int64); err != nil {
			return 0, 0, err
		}

		return borrowerID, nextMemberID.Int64, tx.Commit()
	} else {
		// No one waiting, make available
		if _, err := tx.Exec(`UPDATE books SET available=1, borrower_id=NULL WHERE id=?`, bookID); err != nil {
			return 0, 0, err
		}

		return borrowerID, 0, tx.Commit()
	}
}

func (d *Database) UpdateBookContent(bookID int64, content string) error {
	_, err := d.db.Exec(`UPDATE books SET content=? WHERE id=?`, content, bookID)
	return err
}

func (d *Database) GetMember(id int64) (*Member, error) {
	var m Member
	err := d.db.QueryRow(`SELECT id,name,password_hash FROM members WHERE id=?`, id).
		Scan(&m.ID, &m.Name, &m.PasswordHash)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (d *Database) GetAllMembers() ([]*Member, error) {
	rows, err := d.db.Query(`SELECT id,name,password_hash FROM members ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []*Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.Name, &m.PasswordHash); err != nil {
			return nil, err
		}
		members = append(members, &m)
	}
	return members, rows.Err()
}

func (d *Database) GetReservations(bookID int64) ([]*Member, error) {
	query := `SELECT m.id, m.name, m.password_hash
              FROM reservations r
              JOIN members m ON r.member_id = m.id
              WHERE r.book_id = ? AND r.fulfilled_time IS NULL
              ORDER BY r.reservation_time`

	rows, err := d.db.Query(query, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []*Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.Name, &m.PasswordHash); err != nil {
			return nil, err
		}
		members = append(members, &m)
	}
	return members, rows.Err()
}

func (d *Database) GetMemberReservations(memberID int64) ([]*Book, error) {
	query := `SELECT b.id, b.title, b.author, b.content, b.available, COALESCE(b.borrower_id,0)
              FROM reservations r
              JOIN books b ON r.book_id = b.id
              WHERE r.member_id = ? AND r.fulfilled_time IS NULL
              ORDER BY r.reservation_time`

	rows, err := d.db.Query(query, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []*Book
	for rows.Next() {
		var b Book
		if err := rows.Scan(&b.ID, &b.Title, &b.Author, &b.Content, &b.Available, &b.BorrowerID); err != nil {
			return nil, err
		}
		books = append(books, &b)
	}
	return books, rows.Err()
}

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
		return fmt.Errorf("no active reservation found for this book and member")
	}

	return nil
}

type ReadBookValidation struct {
	BookExists        bool
	BookTitle         string
	BookAuthor        string
	BookAvailable     bool
	BookBorrowerID    int64
	BookContentLength int
	HasContent        bool
	MemberExists      bool
	MemberName        string
	CanAutoCheckout   bool // Book is available for checkout
	CanRead           bool // Member can read (owns book or can auto-checkout)
}

func (d *Database) ValidateReadBookAccess(bookID, memberID int64) (*ReadBookValidation, error) {
	v := &ReadBookValidation{}

	// Check book exists and get details
	var title, author, content string
	var available bool
	var borrowerID sql.NullInt64
	err := d.db.QueryRow(`SELECT title, author, content, available, borrower_id FROM books WHERE id=?`, bookID).
		Scan(&title, &author, &content, &available, &borrowerID)

	if err == sql.ErrNoRows {
		v.BookExists = false
		return v, nil
	}
	if err != nil {
		return nil, err
	}

	v.BookExists = true
	v.BookTitle = title
	v.BookAuthor = author
	v.BookAvailable = available
	if borrowerID.Valid {
		v.BookBorrowerID = borrowerID.Int64
	}
	v.BookContentLength = len(content)
	v.HasContent = len(strings.TrimSpace(content)) > 0

	// Check member exists
	var memberName string
	err = d.db.QueryRow(`SELECT name FROM members WHERE id=?`, memberID).Scan(&memberName)
	if err == sql.ErrNoRows {
		v.MemberExists = false
		return v, nil
	}
	if err != nil {
		return nil, err
	}

	v.MemberExists = true
	v.MemberName = memberName

	// Determine access rights
	v.CanAutoCheckout = available
	v.CanRead = available || (borrowerID.Valid && borrowerID.Int64 == memberID)

	return v, nil
}

func (d *Database) GetBookContentChunk(bookID int64, offset, length int) (string, error) {
	var content string
	err := d.db.QueryRow(`SELECT content FROM books WHERE id=?`, bookID).Scan(&content)
	if err != nil {
		return "", err
	}

	if offset >= len(content) {
		return "", nil
	}

	end := offset + length
	if end > len(content) {
		end = len(content)
	}

	return content[offset:end], nil
}

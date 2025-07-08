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
// Schema migration with proper password support
// ---------------------------------------------------------------------------

const schemaVersion = 3

func applyMigrations(db *sql.DB) error {
	// Create schema_version table if it doesn't exist
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER)`); err != nil {
		return fmt.Errorf("create schema_version table: %w", err)
	}

	// Get current version
	var currentVersion int
	err := db.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&currentVersion)
	if err == sql.ErrNoRows {
		currentVersion = 0
	} else if err != nil {
		return fmt.Errorf("get schema version: %w", err)
	}

	// Apply migrations in sequence
	if currentVersion < 1 {
		if err := applyMigration1(db); err != nil {
			return err
		}
	}
	if currentVersion < 2 {
		if err := applyMigration2(db); err != nil {
			return err
		}
	}
	if currentVersion < 3 {
		if err := applyMigration3(db); err != nil {
			return err
		}
	}

	// Update version
	if currentVersion == 0 {
		if _, err := db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, schemaVersion); err != nil {
			return fmt.Errorf("insert schema version: %w", err)
		}
	} else {
		if _, err := db.Exec(`UPDATE schema_version SET version = ?`, schemaVersion); err != nil {
			return fmt.Errorf("update schema version: %w", err)
		}
	}

	return nil
}

func applyMigration1(db *sql.DB) error {
	// Initial schema
	schema := `
		CREATE TABLE IF NOT EXISTS books (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			author TEXT NOT NULL,
			content TEXT DEFAULT '',
			available BOOLEAN DEFAULT 1,
			borrower_id INTEGER,
			FOREIGN KEY (borrower_id) REFERENCES members(id)
		);

		CREATE TABLE IF NOT EXISTS members (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE
		);

		CREATE TABLE IF NOT EXISTS checkouts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			book_id INTEGER NOT NULL,
			member_id INTEGER NOT NULL,
			checkout_time DATETIME DEFAULT CURRENT_TIMESTAMP,
			return_time DATETIME,
			FOREIGN KEY (book_id) REFERENCES books(id),
			FOREIGN KEY (member_id) REFERENCES members(id)
		);

		CREATE TABLE IF NOT EXISTS reservations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			book_id INTEGER NOT NULL,
			member_id INTEGER NOT NULL,
			reservation_time DATETIME DEFAULT CURRENT_TIMESTAMP,
			fulfilled_time DATETIME,
			FOREIGN KEY (book_id) REFERENCES books(id),
			FOREIGN KEY (member_id) REFERENCES members(id)
		);
	`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("apply migration 1: %w", err)
	}
	return nil
}

func applyMigration2(db *sql.DB) error {
	// Add FTS5 support
	ftsSchema := `
		CREATE VIRTUAL TABLE IF NOT EXISTS books_fts USING fts5(
			title, author, content, content_id UNINDEXED
		);

		-- Populate FTS table with existing data
		INSERT OR IGNORE INTO books_fts(title, author, content, content_id)
		SELECT title, author, content, id FROM books;

		-- Trigger to keep FTS in sync
		CREATE TRIGGER IF NOT EXISTS books_fts_insert AFTER INSERT ON books BEGIN
			INSERT INTO books_fts(title, author, content, content_id) VALUES (new.title, new.author, new.content, new.id);
		END;

		CREATE TRIGGER IF NOT EXISTS books_fts_update AFTER UPDATE ON books BEGIN
			UPDATE books_fts SET title = new.title, author = new.author, content = new.content WHERE content_id = new.id;
		END;

		CREATE TRIGGER IF NOT EXISTS books_fts_delete AFTER DELETE ON books BEGIN
			DELETE FROM books_fts WHERE content_id = old.id;
		END;
	`
	if _, err := db.Exec(ftsSchema); err != nil {
		return fmt.Errorf("apply migration 2: %w", err)
	}
	return nil
}

func applyMigration3(db *sql.DB) error {
	// Add password authentication support with backwards compatibility
	passwordSchema := `
		-- Add password_hash column with backwards compatibility
		ALTER TABLE members ADD COLUMN password_hash TEXT DEFAULT NULL;
	`
	if _, err := db.Exec(passwordSchema); err != nil {
		return fmt.Errorf("apply migration 3: %w", err)
	}
	return nil
}

func (d *Database) prepareStatements() error {
	var err error
	d.addBookStmt, err = d.db.Prepare(`INSERT INTO books(title, author, content) VALUES(?,?,?)`)
	if err != nil {
		return fmt.Errorf("prepare addBookStmt: %w", err)
	}
	d.addMemberStmt, err = d.db.Prepare(`INSERT INTO members(name, password_hash) VALUES(?,?)`)
	if err != nil {
		return fmt.Errorf("prepare addMemberStmt: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Secure Password Management
// ---------------------------------------------------------------------------

const (
	bcryptCost        = 12 // Higher cost for better security
	maxPasswordLength = 72 // bcrypt limit
	minPasswordLength = 1  // Minimum length (can't be empty)
)

// HashPassword securely hashes a password using bcrypt with proper validation
func (d *Database) HashPassword(password string) (string, error) {
	// Validate password length and content
	if strings.TrimSpace(password) == "" {
		return "", fmt.Errorf("password cannot be empty")
	}

	if len(password) < minPasswordLength {
		return "", fmt.Errorf("password must be at least %d character long", minPasswordLength)
	}

	if len(password) > maxPasswordLength {
		return "", fmt.Errorf("password too long (maximum %d characters)", maxPasswordLength)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword verifies a password against its hash using constant-time comparison
func (d *Database) CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// AuthenticateMember verifies member credentials and provides secure error messages
func (d *Database) AuthenticateMember(memberID int64, password string) error {
	var storedHash sql.NullString
	var memberName string

	err := d.db.QueryRow(`SELECT name, password_hash FROM members WHERE id = ?`, memberID).
		Scan(&memberName, &storedHash)

	if err == sql.ErrNoRows {
		// Generic error message - don't reveal if member exists
		return fmt.Errorf("authentication failed: invalid member ID or password")
	}
	if err != nil {
		return fmt.Errorf("database error during authentication: %w", err)
	}

	// Handle legacy members without passwords (backwards compatibility)
	if !storedHash.Valid || storedHash.String == "" {
		return fmt.Errorf("member %s has not set up a password yet. Please contact administrator", memberName)
	}

	// Verify password using constant-time comparison
	if !d.CheckPassword(password, storedHash.String) {
		// Generic error message - don't reveal which part failed
		return fmt.Errorf("authentication failed: invalid member ID or password")
	}

	return nil
}

// ResetMemberPassword securely updates a member's password with proper validation
func (d *Database) ResetMemberPassword(memberID int64, newPassword string) error {
	// Validate new password
	newHash, err := d.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("invalid password: %w", err)
	}

	// Check if member exists
	var memberName string
	err = d.db.QueryRow(`SELECT name FROM members WHERE id = ?`, memberID).Scan(&memberName)
	if err == sql.ErrNoRows {
		return fmt.Errorf("member with ID %d not found", memberID)
	}
	if err != nil {
		return fmt.Errorf("database error: %w", err)
	}

	// Update password
	result, err := d.db.Exec(`UPDATE members SET password_hash = ? WHERE id = ?`, newHash, memberID)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to verify password update: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("member with ID %d not found", memberID)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Member Management with Authentication
// ---------------------------------------------------------------------------

// AddMember creates a new member with proper password validation
func (d *Database) AddMember(name, password string) (int64, error) {
	// Validate inputs
	if strings.TrimSpace(name) == "" {
		return 0, fmt.Errorf("member name cannot be empty")
	}

	// Hash password with validation
	hashedPassword, err := d.HashPassword(password)
	if err != nil {
		return 0, err
	}

	// Insert member
	res, err := d.addMemberStmt.Exec(name, hashedPassword)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return 0, fmt.Errorf("member with name '%s' already exists", name)
		}
		return 0, fmt.Errorf("failed to add member: %w", err)
	}

	return res.LastInsertId()
}

// ---------------------------------------------------------------------------
// Book Management
// ---------------------------------------------------------------------------

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
              JOIN books b ON fts.content_id = b.id
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

// ---------------------------------------------------------------------------
// Circulation with Authorization Checks
// ---------------------------------------------------------------------------

// CheckoutBook performs a book checkout with proper validation
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

	// Verify member exists
	var memberName string
	err = tx.QueryRow(`SELECT name FROM members WHERE id=?`, memberID).Scan(&memberName)
	if err == sql.ErrNoRows {
		return fmt.Errorf("member not found")
	}
	if err != nil {
		return err
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

// ReserveBook implements proper reservation logic with fix for the "already borrowed" bug
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

	// Verify member exists
	var memberName string
	err = tx.QueryRow(`SELECT name FROM members WHERE id=?`, memberID).Scan(&memberName)
	if err == sql.ErrNoRows {
		return fmt.Errorf("member not found")
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

	// CRITICAL FIX: Check if member is the current borrower
	if borrowerID.Valid && borrowerID.Int64 == memberID {
		return fmt.Errorf("you already have this book checked out")
	}

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
// Returns the member ID who returned the book.
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

// VerifyReturnAuthorization checks if a member can return a specific book
func (d *Database) VerifyReturnAuthorization(bookID, memberID int64) error {
	var borrowerID sql.NullInt64
	var available bool
	err := d.db.QueryRow(`SELECT borrower_id, available FROM books WHERE id=?`, bookID).Scan(&borrowerID, &available)
	if err == sql.ErrNoRows {
		return fmt.Errorf("book not found")
	}
	if err != nil {
		return fmt.Errorf("database error: %w", err)
	}

	if available {
		return fmt.Errorf("book is not currently checked out")
	}

	if !borrowerID.Valid || borrowerID.Int64 != memberID {
		return fmt.Errorf("you can only return books that you have checked out")
	}

	return nil
}

func (d *Database) UpdateBookContent(bookID int64, content string) error {
	_, err := d.db.Exec(`UPDATE books SET content=? WHERE id=?`, content, bookID)
	return err
}

func (d *Database) GetMember(id int64) (*Member, error) {
	var m Member
	var passwordHash sql.NullString
	err := d.db.QueryRow(`SELECT id,name,password_hash FROM members WHERE id=?`, id).
		Scan(&m.ID, &m.Name, &passwordHash)
	if err != nil {
		return nil, err
	}

	// Only set password hash if it exists (backwards compatibility)
	if passwordHash.Valid {
		m.PasswordHash = passwordHash.String
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
		var passwordHash sql.NullString
		if err := rows.Scan(&m.ID, &m.Name, &passwordHash); err != nil {
			return nil, err
		}

		// Only set password hash if it exists (backwards compatibility)
		if passwordHash.Valid {
			m.PasswordHash = passwordHash.String
		}

		members = append(members, &m)
	}
	return members, rows.Err()
}

func (d *Database) GetReservations(bookID int64) ([]*Member, error) {
	query := `SELECT m.id, m.name, COALESCE(m.password_hash, '') as password_hash
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
		var passwordHash string
		if err := rows.Scan(&m.ID, &m.Name, &passwordHash); err != nil {
			return nil, err
		}
		m.PasswordHash = passwordHash
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

// ---------------------------------------------------------------------------
// Reading System with Proper Validation
// ---------------------------------------------------------------------------

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
	CanRead           bool // Member can read (owns book or can auto-checkout with content)
}

// ValidateReadBookAccess performs comprehensive validation for reading permissions
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
		// Still need to check if member exists for complete validation
	} else if err != nil {
		return nil, err
	} else {
		v.BookExists = true
		v.BookTitle = title
		v.BookAuthor = author
		v.BookAvailable = available
		if borrowerID.Valid {
			v.BookBorrowerID = borrowerID.Int64
		}
		v.BookContentLength = len(content)
		v.HasContent = len(strings.TrimSpace(content)) > 0
	}

	// Check member exists
	var memberName string
	err = d.db.QueryRow(`SELECT name FROM members WHERE id=?`, memberID).Scan(&memberName)
	if err == sql.ErrNoRows {
		v.MemberExists = false
	} else if err != nil {
		return nil, err
	} else {
		v.MemberExists = true
		v.MemberName = memberName
	}

	// Determine access rights - fix the logic flaws from Sonnet
	if v.BookExists && v.MemberExists {
		v.CanAutoCheckout = available && v.HasContent
		// FIXED: CanRead should only be true if there's content AND either available or member owns it
		v.CanRead = v.HasContent && (available || (borrowerID.Valid && borrowerID.Int64 == memberID))
	}

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

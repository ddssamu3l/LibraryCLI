package library

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LibraryManager is a thin façade over the Database, keeping CLI code simple.
type LibraryManager struct {
	db *Database
}

// NewLibraryManager opens (or creates) the SQLite database at dbPath.
func NewLibraryManager(dbPath string) (*LibraryManager, error) {
	db, err := NewDatabase(dbPath)
	if err != nil {
		return nil, err
	}
	return &LibraryManager{db: db}, nil
}

// Close closes the underlying database.
func (lm *LibraryManager) Close() error { return lm.db.Close() }

// ------------------ Book helpers ------------------

func (lm *LibraryManager) AddBook(title, author string) (int64, error) {
	return lm.db.AddBook(title, author, "")
}

// AddBookFromFile reads the file at path (relative paths resolve from cwd) and stores it.
func (lm *LibraryManager) AddBookFromFile(title, author, path string) (int64, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return lm.db.AddBookFromReader(title, author, f)
}

func (lm *LibraryManager) UpdateBookContent(id int64, content string) error {
	return lm.db.UpdateBookContent(id, content)
}

func (lm *LibraryManager) GetBook(id int64) (*Book, error) { return lm.db.GetBook(id) }
func (lm *LibraryManager) GetAllBooks() ([]*Book, error)   { return lm.db.GetAllBooks() }

// ------------------ Member helpers ------------------

func (lm *LibraryManager) AddMember(name string) (int64, error) { return lm.db.AddMember(name) }
func (lm *LibraryManager) GetMember(id int64) (*Member, error)  { return lm.db.GetMember(id) }
func (lm *LibraryManager) GetAllMembers() ([]*Member, error)    { return lm.db.GetAllMembers() }

// ------------------ Reservation helpers ------------------

func (lm *LibraryManager) ReserveBook(bookID, memberID int64) error {
	return lm.db.ReserveBook(bookID, memberID)
}

func (lm *LibraryManager) GetReservations(bookID int64) ([]*Member, error) {
	return lm.db.GetReservations(bookID)
}

func (lm *LibraryManager) GetMemberReservations(memberID int64) ([]*Book, error) {
	return lm.db.GetMemberReservations(memberID)
}

func (lm *LibraryManager) CancelReservation(bookID, memberID int64) error {
	return lm.db.CancelReservation(bookID, memberID)
}

// ------------------ Search ------------------

func (lm *LibraryManager) SearchBooks(q string) ([]*Book, error) {
	return lm.db.SearchBooks(q)
}

// ------------------ Circulation ------------------

func (lm *LibraryManager) CheckoutBook(bookID, memberID int64) error {
	return lm.db.CheckoutBook(bookID, memberID)
}

// ReturnBook returns the book and yields the member who had it.
func (lm *LibraryManager) ReturnBook(bookID int64) (int64, error) {
	return lm.db.ReturnBook(bookID)
}

// ReturnBookWithDetails returns the book and provides detailed information about what happened.
func (lm *LibraryManager) ReturnBookWithDetails(bookID int64) (returnedByMemberID int64, assignedToMemberID int64, err error) {
	// First get the current borrower
	book, err := lm.db.GetBook(bookID)
	if err != nil {
		return 0, 0, err
	}
	if book.Available {
		return 0, 0, fmt.Errorf("book %d is not checked out", bookID)
	}

	// Check if there are any reservations
	reservations, err := lm.db.GetReservations(bookID)
	if err != nil {
		return 0, 0, err
	}

	// Perform the return
	returnedBy, err := lm.db.ReturnBook(bookID)
	if err != nil {
		return 0, 0, err
	}

	// Check if book was automatically assigned
	bookAfter, err := lm.db.GetBook(bookID)
	if err != nil {
		return returnedBy, 0, nil // Return succeeded but can't check assignment
	}

	if !bookAfter.Available && len(reservations) > 0 {
		// Book was assigned to next person in queue
		return returnedBy, bookAfter.BorrowerID, nil
	}

	// Book became available (no reservations)
	return returnedBy, 0, nil
}

// ------------------ Legacy no-ops ------------------

func (lm *LibraryManager) SaveData(string) error { return nil }
func (lm *LibraryManager) LoadData(string) error { return nil }

// ------------------ Utilities ------------------

// PrettyBook formats a book for lists.
func PrettyBook(b *Book, borrowerName string) string {
	return fmt.Sprintf("%-5d %-30s %-25s %-10t %-25s", b.ID, b.Title, b.Author, b.Available, borrowerName)
}

// UpdateBookContentFromFile streams text from a file and updates the book's content.
func (lm *LibraryManager) UpdateBookContentFromFile(id int64, path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("file path cannot be empty")
	}
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return err
	}
	defer f.Close()
	var sb strings.Builder
	if _, err := io.Copy(&sb, f); err != nil {
		return err
	}
	return lm.db.UpdateBookContent(id, sb.String())
}

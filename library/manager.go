package library

import (
	"bufio"
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

// ------------------ Member helpers with Authentication ------------------

// AddMember creates a new member with password validation
func (lm *LibraryManager) AddMember(name, password string) (int64, error) {
	return lm.db.AddMember(name, password)
}

func (lm *LibraryManager) GetMember(id int64) (*Member, error) { return lm.db.GetMember(id) }
func (lm *LibraryManager) GetAllMembers() ([]*Member, error)   { return lm.db.GetAllMembers() }

// AuthenticateMember verifies member credentials
func (lm *LibraryManager) AuthenticateMember(memberID int64, password string) error {
	return lm.db.AuthenticateMember(memberID, password)
}

// ResetMemberPassword updates a member's password with validation
func (lm *LibraryManager) ResetMemberPassword(memberID int64, newPassword string) error {
	return lm.db.ResetMemberPassword(memberID, newPassword)
}

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

// ------------------ Circulation with Authorization ------------------

// CheckoutBook performs a book checkout
func (lm *LibraryManager) CheckoutBook(bookID, memberID int64) error {
	return lm.db.CheckoutBook(bookID, memberID)
}

// ReturnBook returns the book and yields the member who had it with authorization check
func (lm *LibraryManager) ReturnBook(bookID, memberID int64) (int64, error) {
	// First verify the member is authorized to return this book
	if err := lm.db.VerifyReturnAuthorization(bookID, memberID); err != nil {
		return 0, err
	}

	return lm.db.ReturnBook(bookID)
}

// ReturnBookWithDetails returns the book and provides detailed information about what happened
func (lm *LibraryManager) ReturnBookWithDetails(bookID, memberID int64) (returnedByMemberID int64, assignedToMemberID int64, err error) {
	// First verify the member is authorized to return this book
	if err := lm.db.VerifyReturnAuthorization(bookID, memberID); err != nil {
		return 0, 0, err
	}

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

// ReadBook allows a member to read a book with pagination and proper authorization
// Only allows reading if the book is already checked out to the member.
func (lm *LibraryManager) ReadBook(bookID, memberID int64) error {
	// Single optimized query for all validation
	validation, err := lm.db.ValidateReadBookAccess(bookID, memberID)
	if err != nil {
		return fmt.Errorf("database error: %w", err)
	}

	// Check validation results with improved error messages
	if !validation.BookExists {
		return fmt.Errorf("book not found")
	}

	if !validation.MemberExists {
		return fmt.Errorf("member not found")
	}

	if !validation.HasContent {
		return fmt.Errorf("book has no content to read")
	}

	// Additional validation: check for whitespace-only content using Go's more robust trimming
	// This handles edge cases where SQLite TRIM might not catch all whitespace types
	if validation.BookContentLength > 0 {
		// Get a small sample of content to check if it's all whitespace
		sampleContent, err := lm.db.GetBookContentChunk(bookID, 0, 1000) // Check first 1000 chars
		if err != nil {
			return fmt.Errorf("failed to validate content: %w", err)
		}
		if strings.TrimSpace(sampleContent) == "" {
			return fmt.Errorf("book has no content to read")
		}
	}

	// Check if member can read the book (must already have it checked out)
	if !validation.CanRead {
		if validation.BookAvailable {
			return fmt.Errorf("book is available but not checked out to you. Please check out the book first to read it")
		} else {
			// Book is checked out by someone else - don't expose borrower information
			return fmt.Errorf("book is currently checked out by another member")
		}
	}

	// Start the reading interface with efficient pagination
	return lm.startReadingInterface(bookID, validation.BookTitle, validation.BookAuthor,
		validation.MemberName, validation.BookContentLength)
}

// startReadingInterface provides a paginated reading experience with lazy loading
func (lm *LibraryManager) startReadingInterface(bookID int64, title, author, memberName string, totalLength int) error {
	const pageSize = 1500

	// Calculate total pages
	totalPages := (totalLength + pageSize - 1) / pageSize
	if totalPages == 0 {
		return fmt.Errorf("book has no content to display")
	}

	currentPage := 0
	scanner := bufio.NewScanner(os.Stdin)

	// Clear screen and show initial page
	fmt.Print("\033[2J\033[H") // Clear screen and move cursor to top

	for {
		// Lazy load current page content
		offset := currentPage * pageSize
		pageContent, err := lm.db.GetBookContentChunk(bookID, offset, pageSize)
		if err != nil {
			return fmt.Errorf("failed to load page content: %w", err)
		}

		// Display header
		fmt.Printf("═══════════════════════════════════════════════════════════════════════════════\n")
		fmt.Printf("📖 %s by %s\n", title, author)
		fmt.Printf("Reader: %s | Page %d of %d\n", memberName, currentPage+1, totalPages)
		fmt.Printf("═══════════════════════════════════════════════════════════════════════════════\n\n")

		// Display current page content
		fmt.Println(pageContent)

		// Display navigation footer (only show navigation for multi-page books)
		fmt.Printf("\n═══════════════════════════════════════════════════════════════════════════════\n")
		if totalPages == 1 {
			fmt.Printf("📖 End of book. Press [q] to quit.")
		} else {
			fmt.Printf("📖 Navigation: [n]ext | [p]revious | [g]oto page | [q]uit")
		}
		fmt.Printf("\n═══════════════════════════════════════════════════════════════════════════════\n")
		fmt.Print("Command: ")

		if !scanner.Scan() {
			break // EOF or error
		}

		input := strings.ToLower(strings.TrimSpace(scanner.Text()))
		fmt.Print("\033[2J\033[H") // Clear screen

		switch input {
		case "n", "next":
			if totalPages == 1 {
				fmt.Println("📖 This book has only one page!")
				fmt.Println("Press Enter to continue...")
				scanner.Scan()
				fmt.Print("\033[2J\033[H")
			} else if currentPage < totalPages-1 {
				currentPage++
			} else {
				fmt.Println("📖 You're already on the last page!")
				fmt.Println("Press Enter to continue...")
				scanner.Scan()
				fmt.Print("\033[2J\033[H")
			}
		case "p", "prev", "previous":
			if totalPages == 1 {
				fmt.Println("📖 This book has only one page!")
				fmt.Println("Press Enter to continue...")
				scanner.Scan()
				fmt.Print("\033[2J\033[H")
			} else if currentPage > 0 {
				currentPage--
			} else {
				fmt.Println("📖 You're already on the first page!")
				fmt.Println("Press Enter to continue...")
				scanner.Scan()
				fmt.Print("\033[2J\033[H")
			}
		case "g", "goto":
			if totalPages == 1 {
				fmt.Println("📖 This book has only one page!")
				fmt.Println("Press Enter to continue...")
				scanner.Scan()
				fmt.Print("\033[2J\033[H")
			} else {
				fmt.Printf("Enter page number (1-%d): ", totalPages)
				if scanner.Scan() {
					var pageNum int
					if n, err := fmt.Sscanf(scanner.Text(), "%d", &pageNum); err == nil && n == 1 {
						pageNum-- // Convert to 0-based index
						if pageNum < 0 {
							pageNum = 0
						} else if pageNum >= totalPages {
							pageNum = totalPages - 1
						}
						currentPage = pageNum
					} else {
						fmt.Println("Invalid page number!")
						fmt.Println("Press Enter to continue...")
						scanner.Scan()
					}
				}
				fmt.Print("\033[2J\033[H")
			}
		case "q", "quit", "exit":
			fmt.Printf("📖 Finished reading '%s'.\n", title)
			return nil
		case "":
			// Just refresh the display
			continue
		default:
			fmt.Printf("Unknown command: %s\n", input)
			if totalPages == 1 {
				fmt.Println("Use: [q]uit")
			} else {
				fmt.Println("Use: [n]ext, [p]revious, [g]oto, or [q]uit")
			}
			fmt.Println("Press Enter to continue...")
			scanner.Scan()
			fmt.Print("\033[2J\033[H")
		}
	}

	return nil
}

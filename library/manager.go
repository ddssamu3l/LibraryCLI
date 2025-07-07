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

// ReadBook allows a member to read a book with pagination.
// If the book is available, it checks it out first.
// If the book is checked out, only allows reading if it's checked out by the same member.
func (lm *LibraryManager) ReadBook(bookID, memberID int64) error {
	// Get book details
	book, err := lm.db.GetBook(bookID)
	if err != nil {
		return fmt.Errorf("book not found: %w", err)
	}

	// Verify member exists
	member, err := lm.db.GetMember(memberID)
	if err != nil {
		return fmt.Errorf("member not found: %w", err)
	}

	// Check access permissions and handle checkout if needed
	if book.Available {
		// Book is available, check it out for the member
		if err := lm.db.CheckoutBook(bookID, memberID); err != nil {
			return fmt.Errorf("failed to check out book: %w", err)
		}
		fmt.Printf("Book '%s' checked out to %s for reading.\n", book.Title, member.Name)
	} else if book.BorrowerID != memberID {
		// Book is checked out by someone else
		borrower, _ := lm.db.GetMember(book.BorrowerID)
		borrowerName := "Unknown"
		if borrower != nil {
			borrowerName = borrower.Name
		}
		return fmt.Errorf("book is currently checked out by %s (ID: %d)", borrowerName, book.BorrowerID)
	}
	// If book.BorrowerID == memberID, member already has the book checked out

	// Check if book has content
	if strings.TrimSpace(book.Content) == "" {
		return fmt.Errorf("book has no content to read")
	}

	// Start the reading interface
	return lm.startReadingInterface(book, member)
}

// startReadingInterface provides a paginated reading experience.
func (lm *LibraryManager) startReadingInterface(book *Book, member *Member) error {
	const pageSize = 1500
	content := book.Content

	// Split content into pages
	pages := make([]string, 0)
	for i := 0; i < len(content); i += pageSize {
		end := i + pageSize
		if end > len(content) {
			end = len(content)
		}
		pages = append(pages, content[i:end])
	}

	if len(pages) == 0 {
		return fmt.Errorf("book has no content to display")
	}

	currentPage := 0
	scanner := bufio.NewScanner(os.Stdin)

	// Clear screen and show initial page
	fmt.Print("\033[2J\033[H") // Clear screen and move cursor to top

	for {
		// Display header
		fmt.Printf("═══════════════════════════════════════════════════════════════════════════════\n")
		fmt.Printf("📖 %s by %s\n", book.Title, book.Author)
		fmt.Printf("Reader: %s | Page %d of %d\n", member.Name, currentPage+1, len(pages))
		fmt.Printf("═══════════════════════════════════════════════════════════════════════════════\n\n")

		// Display current page content
		fmt.Println(pages[currentPage])

		// Display navigation footer
		fmt.Printf("\n═══════════════════════════════════════════════════════════════════════════════\n")
		fmt.Printf("Navigation: [n]ext | [p]revious | [g]oto page | [q]uit\n")
		if currentPage > 0 {
			fmt.Printf("← Previous")
		}
		if currentPage < len(pages)-1 {
			if currentPage > 0 {
				fmt.Printf(" | ")
			}
			fmt.Printf("Next →")
		}
		fmt.Printf("\n> ")

		// Get user input
		if !scanner.Scan() {
			break
		}

		input := strings.ToLower(strings.TrimSpace(scanner.Text()))

		// Clear screen for next display
		fmt.Print("\033[2J\033[H")

		switch input {
		case "n", "next":
			if currentPage < len(pages)-1 {
				currentPage++
			} else {
				fmt.Println("📖 You're already on the last page!")
				fmt.Println("Press Enter to continue...")
				scanner.Scan()
				fmt.Print("\033[2J\033[H")
			}
		case "p", "prev", "previous":
			if currentPage > 0 {
				currentPage--
			} else {
				fmt.Println("📖 You're already on the first page!")
				fmt.Println("Press Enter to continue...")
				scanner.Scan()
				fmt.Print("\033[2J\033[H")
			}
		case "g", "goto":
			fmt.Printf("Enter page number (1-%d): ", len(pages))
			if scanner.Scan() {
				if pageNum, err := fmt.Sscanf(scanner.Text(), "%d", &currentPage); err == nil && pageNum == 1 {
					currentPage-- // Convert to 0-based index
					if currentPage < 0 {
						currentPage = 0
					} else if currentPage >= len(pages) {
						currentPage = len(pages) - 1
					}
				} else {
					fmt.Println("Invalid page number!")
					fmt.Println("Press Enter to continue...")
					scanner.Scan()
				}
			}
			fmt.Print("\033[2J\033[H")
		case "q", "quit", "exit":
			fmt.Printf("📖 Finished reading '%s'. The book remains checked out to you.\n", book.Title)
			return nil
		case "":
			// Just refresh the display
			continue
		default:
			fmt.Printf("Unknown command: %s\n", input)
			fmt.Println("Use: [n]ext, [p]revious, [g]oto, or [q]uit")
			fmt.Println("Press Enter to continue...")
			scanner.Scan()
			fmt.Print("\033[2J\033[H")
		}
	}

	return nil
}

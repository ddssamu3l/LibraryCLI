package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"library-management/library"

	"golang.org/x/term"
)

const dbFile = "library.db"

func main() {
	manager, err := library.NewLibraryManager(dbFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer manager.Close()

	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("Welcome to the Library Management System (SQLite edition)!")
	fmt.Println("Available commands: add book, add member, list books, list members, search book, checkout, return, reserve, list reservations, cancel reservation, update content, read book, reset password, exit")

	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}
		cmd := strings.TrimSpace(scanner.Text())

		switch cmd {
		case "add book":
			handleAddBook(scanner, manager)
		case "add member":
			handleAddMember(scanner, manager)
		case "list books":
			handleListBooks(manager)
		case "list members":
			handleListMembers(manager)
		case "search book":
			handleSearchBooks(scanner, manager)
		case "checkout":
			handleCheckout(scanner, manager)
		case "return":
			handleReturn(scanner, manager)
		case "reserve":
			handleReserve(scanner, manager)
		case "list reservations":
			handleListReservations(scanner, manager)
		case "cancel reservation":
			handleCancelReservation(scanner, manager)
		case "update content":
			handleUpdateContent(scanner, manager)
		case "read book":
			handleReadBook(scanner, manager)
		case "reset password":
			handleResetPassword(scanner, manager)
		case "exit":
			fmt.Println("Goodbye!")
			return
		default:
			fmt.Println("Unknown command.")
		}
	}
}

func handleAddBook(sc *bufio.Scanner, mgr *library.LibraryManager) {
	fmt.Print("Title: ")
	if !sc.Scan() {
		return
	}
	title := strings.TrimSpace(sc.Text())

	fmt.Print("Author: ")
	if !sc.Scan() {
		return
	}
	author := strings.TrimSpace(sc.Text())

	fmt.Print("Content file path (or 'manual' to type content): ")
	if !sc.Scan() {
		return
	}
	input := strings.TrimSpace(sc.Text())

	var id int64
	var err error

	if input == "manual" {
		fmt.Print("Content: ")
		if !sc.Scan() {
			return
		}
		content := sc.Text()
		id, err = mgr.AddBook(title, author)
		if err == nil && content != "" {
			err = mgr.UpdateBookContent(id, content)
		}
	} else {
		// Try to read from file
		if !filepath.IsAbs(input) {
			input = filepath.Join(".", input)
		}
		id, err = mgr.AddBookFromFile(title, author, input)
	}

	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Added book with ID %d\n", id)
	}
}

func handleAddMember(sc *bufio.Scanner, mgr *library.LibraryManager) {
	fmt.Print("Name: ")
	if !sc.Scan() {
		return
	}
	name := strings.TrimSpace(sc.Text())

	fmt.Print("Password: ")
	password, err := readPassword()
	if err != nil {
		fmt.Printf("Error reading password: %v\n", err)
		return
	}

	if strings.TrimSpace(password) == "" {
		fmt.Println("Password cannot be empty.")
		return
	}

	id, err := mgr.AddMember(name, password)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Added member with ID %d\n", id)
	}
}

func handleResetPassword(sc *bufio.Scanner, mgr *library.LibraryManager) {
	fmt.Print("Member ID: ")
	if !sc.Scan() {
		return
	}
	memberIDStr := strings.TrimSpace(sc.Text())
	memberID, err := strconv.ParseInt(memberIDStr, 10, 64)
	if err != nil {
		fmt.Println("Invalid member ID.")
		return
	}

	fmt.Print("New Password: ")
	password, err := readPassword()
	if err != nil {
		fmt.Printf("Error reading password: %v\n", err)
		return
	}

	if strings.TrimSpace(password) == "" {
		fmt.Println("Password cannot be empty.")
		return
	}

	if err := mgr.ResetMemberPassword(memberID, password); err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Println("Password reset successfully.")
	}
}

// Authentication helper for member operations
func authenticateMember(sc *bufio.Scanner, mgr *library.LibraryManager) (int64, error) {
	fmt.Print("Member ID: ")
	if !sc.Scan() {
		return 0, fmt.Errorf("failed to read member ID")
	}
	memberIDStr := strings.TrimSpace(sc.Text())
	memberID, err := strconv.ParseInt(memberIDStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid member ID")
	}

	fmt.Print("Password: ")
	password, err := readPassword()
	if err != nil {
		return 0, fmt.Errorf("error reading password: %w", err)
	}

	if err := mgr.AuthenticateMember(memberID, password); err != nil {
		return 0, fmt.Errorf("authentication failed: %w", err)
	}

	return memberID, nil
}

// Updated member-oriented functions with authentication

func handleCheckout(sc *bufio.Scanner, mgr *library.LibraryManager) {
	fmt.Print("Book ID: ")
	if !sc.Scan() {
		return
	}
	bookIDStr := strings.TrimSpace(sc.Text())
	bookID, err := strconv.ParseInt(bookIDStr, 10, 64)
	if err != nil {
		fmt.Println("Invalid book ID.")
		return
	}

	fmt.Println("Member authentication required:")
	memberID, err := authenticateMember(sc, mgr)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if err := mgr.CheckoutBook(bookID, memberID); err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Println("Checked out successfully.")
	}
}

func handleReturn(sc *bufio.Scanner, mgr *library.LibraryManager) {
	fmt.Print("Book ID: ")
	if !sc.Scan() {
		return
	}
	bookIDStr := strings.TrimSpace(sc.Text())
	bookID, err := strconv.ParseInt(bookIDStr, 10, 64)
	if err != nil {
		fmt.Println("Invalid book ID.")
		return
	}

	// Get book info to verify who has it checked out
	book, err := mgr.GetBook(bookID)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	if book.Available {
		fmt.Println("Error: Book is not checked out.")
		return
	}

	fmt.Println("Member authentication required (must be the member who checked out the book):")
	memberID, err := authenticateMember(sc, mgr)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Verify the authenticated member is the one who has the book
	if book.BorrowerID != memberID {
		fmt.Println("Error: You can only return books that you have checked out.")
		return
	}

	returnedBy, assignedTo, err := mgr.ReturnBookWithDetails(bookID)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Book returned by member %d.\n", returnedBy)
		if assignedTo > 0 {
			fmt.Printf("Book automatically assigned to member %d (next in reservation queue).\n", assignedTo)
		} else {
			fmt.Println("Book is now available for checkout.")
		}
	}
}

func handleReserve(sc *bufio.Scanner, mgr *library.LibraryManager) {
	fmt.Print("Book ID: ")
	if !sc.Scan() {
		return
	}
	bookIDStr := strings.TrimSpace(sc.Text())
	bookID, err := strconv.ParseInt(bookIDStr, 10, 64)
	if err != nil {
		fmt.Println("Invalid book ID.")
		return
	}

	fmt.Println("Member authentication required:")
	memberID, err := authenticateMember(sc, mgr)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if err := mgr.ReserveBook(bookID, memberID); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Check status to inform user
	book, err := mgr.GetBook(bookID)
	if err != nil {
		fmt.Println("Reserved successfully.")
		return
	}
	if !book.Available && book.BorrowerID == memberID {
		fmt.Println("Book was available and has been checked out immediately.")
	} else {
		fmt.Println("Book reserved successfully. You will be notified when it becomes available.")
	}
}

func handleListReservations(sc *bufio.Scanner, mgr *library.LibraryManager) {
	fmt.Println("1. List reservations for a book (admin)")
	fmt.Println("2. List your reservations (member)")
	fmt.Print("Choose option (1-2): ")

	if !sc.Scan() {
		return
	}
	option := strings.TrimSpace(sc.Text())

	switch option {
	case "1":
		// Admin function - no authentication required
		fmt.Print("Book ID: ")
		if !sc.Scan() {
			return
		}
		bookIDStr := strings.TrimSpace(sc.Text())
		bookID, err := strconv.ParseInt(bookIDStr, 10, 64)
		if err != nil {
			fmt.Println("Invalid book ID.")
			return
		}

		reservations, err := mgr.GetReservations(bookID)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		if len(reservations) == 0 {
			fmt.Println("No reservations for this book.")
			return
		}

		fmt.Printf("Reservations for book %d (in queue order):\n", bookID)
		for i, m := range reservations {
			fmt.Printf("%d. Member %d: %s\n", i+1, m.ID, m.Name)
		}

	case "2":
		// Member function - requires authentication
		fmt.Println("Member authentication required:")
		memberID, err := authenticateMember(sc, mgr)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		reservations, err := mgr.GetMemberReservations(memberID)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		if len(reservations) == 0 {
			fmt.Println("No active reservations for you.")
			return
		}

		fmt.Printf("Your active reservations:\n")
		for _, b := range reservations {
			fmt.Printf("Book %d: %s by %s\n", b.ID, b.Title, b.Author)
		}

	default:
		fmt.Println("Invalid option.")
	}
}

func handleCancelReservation(sc *bufio.Scanner, mgr *library.LibraryManager) {
	fmt.Print("Book ID: ")
	if !sc.Scan() {
		return
	}
	bookIDStr := strings.TrimSpace(sc.Text())
	bookID, err := strconv.ParseInt(bookIDStr, 10, 64)
	if err != nil {
		fmt.Println("Invalid book ID.")
		return
	}

	fmt.Println("Member authentication required:")
	memberID, err := authenticateMember(sc, mgr)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if err := mgr.CancelReservation(bookID, memberID); err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Println("Reservation cancelled successfully.")
	}
}

func handleReadBook(sc *bufio.Scanner, mgr *library.LibraryManager) {
	fmt.Print("Book ID: ")
	if !sc.Scan() {
		return
	}
	bookIDStr := strings.TrimSpace(sc.Text())
	bookID, err := strconv.ParseInt(bookIDStr, 10, 64)
	if err != nil {
		fmt.Println("Invalid book ID.")
		return
	}

	fmt.Println("Member authentication required:")
	memberID, err := authenticateMember(sc, mgr)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if err := mgr.ReadBook(bookID, memberID); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

// Utility function to read password securely
func readPassword() (string, error) {
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", err
	}
	fmt.Println() // Print newline after password input
	return string(bytePassword), nil
}

func handleListBooks(mgr *library.LibraryManager) {
	books, err := mgr.GetAllBooks()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if len(books) == 0 {
		fmt.Println("No books in the library.")
		return
	}

	fmt.Println("Books in the library:")
	for _, book := range books {
		status := "Available"
		if !book.Available {
			status = fmt.Sprintf("Checked out by member %d", book.BorrowerID)
		}
		fmt.Printf("ID %d: %s by %s (%s)\n", book.ID, book.Title, book.Author, status)
	}
}

func handleListMembers(mgr *library.LibraryManager) {
	members, err := mgr.GetAllMembers()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if len(members) == 0 {
		fmt.Println("No members in the library.")
		return
	}

	fmt.Println("Library members:")
	for _, member := range members {
		fmt.Printf("ID %d: %s\n", member.ID, member.Name)
	}
}

func handleSearchBooks(sc *bufio.Scanner, mgr *library.LibraryManager) {
	fmt.Print("Search query: ")
	if !sc.Scan() {
		return
	}
	query := strings.TrimSpace(sc.Text())

	if query == "" {
		fmt.Println("Search query cannot be empty.")
		return
	}

	books, err := mgr.SearchBooks(query)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if len(books) == 0 {
		fmt.Println("No books found matching your query.")
		return
	}

	fmt.Printf("Found %d book(s) matching '%s':\n", len(books), query)
	for _, book := range books {
		status := "Available"
		if !book.Available {
			status = fmt.Sprintf("Checked out by member %d", book.BorrowerID)
		}
		fmt.Printf("ID %d: %s by %s (%s)\n", book.ID, book.Title, book.Author, status)
	}
}

func handleUpdateContent(sc *bufio.Scanner, mgr *library.LibraryManager) {
	fmt.Print("Book ID: ")
	if !sc.Scan() {
		return
	}
	bookIDStr := strings.TrimSpace(sc.Text())
	bookID, err := strconv.ParseInt(bookIDStr, 10, 64)
	if err != nil {
		fmt.Println("Invalid book ID.")
		return
	}

	fmt.Print("New content file path (or 'manual' to type content): ")
	if !sc.Scan() {
		return
	}
	input := strings.TrimSpace(sc.Text())

	var content string

	if input == "manual" {
		fmt.Print("New content: ")
		if !sc.Scan() {
			return
		}
		content = sc.Text()
	} else {
		// Try to read from file
		if !filepath.IsAbs(input) {
			input = filepath.Join(".", input)
		}
		data, err := os.ReadFile(input)
		if err != nil {
			fmt.Printf("Error reading file: %v\n", err)
			return
		}
		content = string(data)
	}

	if err := mgr.UpdateBookContent(bookID, content); err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Println("Book content updated successfully.")
	}
}

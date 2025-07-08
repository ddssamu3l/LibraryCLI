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

// readPassword securely reads a password with masking
func readPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", err
	}
	fmt.Println() // Add newline after password input
	return strings.TrimSpace(string(bytePassword)), nil
}

// authenticateUser prompts for and verifies user credentials
func authenticateUser(sc *bufio.Scanner, mgr *library.LibraryManager, memberID int64) error {
	password, err := readPassword("Enter your password: ")
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}

	if err := mgr.AuthenticateMember(memberID, password); err != nil {
		return err
	}

	return nil
}

func main() {
	manager, err := library.NewLibraryManager(dbFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer manager.Close()

	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("Welcome to the Library Management System with Secure Authentication!")
	fmt.Println("Available commands:")
	fmt.Println("  Books: add book, list books, search book, update content")
	fmt.Println("  Members: add member, list members, reset password")
	fmt.Println("  Circulation: checkout, return, reserve, list reservations, cancel reservation")
	fmt.Println("  Reading: read book")
	fmt.Println("  System: exit")
	fmt.Println()
	fmt.Println("Tips:")
	fmt.Println("  • For 'list reservations': Enter a Book ID for specific book, or press Enter to see all books")

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
			fmt.Println("Unknown command. Type one of the available commands listed above.")
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

	fmt.Print("Path to text file (optional): ")
	if !sc.Scan() {
		return
	}
	path := strings.TrimSpace(sc.Text())

	var (
		id  int64
		err error
	)

	if path == "" {
		// No content yet
		id, err = mgr.AddBook(title, author)
	} else {
		if _, errStat := os.Stat(filepath.Clean(path)); errStat != nil {
			fmt.Printf("File error: %v. Adding book without content.\n", errStat)
			id, err = mgr.AddBook(title, author)
		} else {
			id, err = mgr.AddBookFromFile(title, author, path)
		}
	}

	if err != nil {
		fmt.Printf("Error adding book: %v\n", err)
	} else {
		if path == "" {
			fmt.Printf("Added book ID %d (no content). Use 'update content' later.\n", id)
		} else {
			fmt.Printf("Added book ID %d with content.\n", id)
		}
	}
}

func handleAddMember(sc *bufio.Scanner, mgr *library.LibraryManager) {
	fmt.Print("Name: ")
	if !sc.Scan() {
		return
	}
	name := strings.TrimSpace(sc.Text())

	password, err := readPassword(fmt.Sprintf("Enter password for %s: ", name))
	if err != nil {
		fmt.Printf("Error reading password: %v\n", err)
		return
	}

	if strings.TrimSpace(password) == "" {
		fmt.Println("Error: Password cannot be empty")
		return
	}

	id, err := mgr.AddMember(name, password)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Added member '%s' with ID %d\n", name, id)
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
		fmt.Printf("Invalid member ID: %s\n", memberIDStr)
		return
	}

	// Verify member exists and get their name
	member, err := mgr.GetMember(memberID)
	if err != nil {
		fmt.Printf("Error: Member with ID %d not found\n", memberID)
		return
	}

	newPassword, err := readPassword(fmt.Sprintf("Enter new password for %s (ID: %d): ", member.Name, memberID))
	if err != nil {
		fmt.Printf("Error reading password: %v\n", err)
		return
	}

	if strings.TrimSpace(newPassword) == "" {
		fmt.Println("Error: Password cannot be empty")
		return
	}

	if err := mgr.ResetMemberPassword(memberID, newPassword); err != nil {
		fmt.Printf("Error resetting password: %v\n", err)
		return
	}

	fmt.Printf("Password successfully reset for %s (ID: %d)\n", member.Name, memberID)
}

func handleListBooks(mgr *library.LibraryManager) {
	books, err := mgr.GetAllBooks()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	if len(books) == 0 {
		fmt.Println("No books in library.")
		return
	}

	fmt.Printf("%-5s %-30s %-25s %-10s %-20s %s\n", "ID", "Title", "Author", "Available", "Borrower", "Reservation Queue")
	fmt.Println(strings.Repeat("-", 120))

	for _, b := range books {
		// Get borrower information
		var borrowerInfo string
		if b.Available {
			borrowerInfo = "None"
		} else {
			if member, err := mgr.GetMember(b.BorrowerID); err == nil {
				borrowerInfo = fmt.Sprintf("%s (ID: %d)", member.Name, member.ID)
			} else {
				borrowerInfo = fmt.Sprintf("ID: %d", b.BorrowerID)
			}
		}

		// Get reservation queue
		reservations, err := mgr.GetReservations(b.ID)
		var queueInfo string
		if err != nil || len(reservations) == 0 {
			queueInfo = "None"
		} else {
			var queueMembers []string
			for i, member := range reservations {
				queueMembers = append(queueMembers, fmt.Sprintf("%d. %s (ID: %d)", i+1, member.Name, member.ID))
			}
			queueInfo = strings.Join(queueMembers, ", ")
		}

		// Print book information
		availStr := "Yes"
		if !b.Available {
			availStr = "No"
		}

		fmt.Printf("%-5d %-30s %-25s %-10s %-20s %s\n",
			b.ID,
			truncateString(b.Title, 30),
			truncateString(b.Author, 25),
			availStr,
			truncateString(borrowerInfo, 20),
			queueInfo)
	}
}

func handleListMembers(mgr *library.LibraryManager) {
	members, err := mgr.GetAllMembers()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if len(members) == 0 {
		fmt.Println("No members registered.")
		return
	}

	fmt.Printf("%-5s %-30s %-15s\n", "ID", "Name", "Password Set")
	fmt.Println(strings.Repeat("-", 55))

	for _, member := range members {
		passwordStatus := "No"
		if member.PasswordHash != "" {
			passwordStatus = "Yes"
		}
		fmt.Printf("%-5d %-30s %-15s\n", member.ID, member.Name, passwordStatus)
	}
}

func handleSearchBooks(sc *bufio.Scanner, mgr *library.LibraryManager) {
	fmt.Print("Query: ")
	if !sc.Scan() {
		return
	}
	query := strings.TrimSpace(sc.Text())

	books, err := mgr.SearchBooks(query)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if len(books) == 0 {
		fmt.Printf("No books found matching '%s'.\n", query)
		return
	}

	fmt.Printf("Found %d book(s) matching '%s':\n", len(books), query)
	fmt.Printf("%-5s %-30s %-25s %-10s %-25s\n", "ID", "Title", "Author", "Available", "Borrower")
	fmt.Println(strings.Repeat("-", 100))

	for _, book := range books {
		borrowerName := ""
		if !book.Available && book.BorrowerID > 0 {
			if member, err := mgr.GetMember(book.BorrowerID); err == nil {
				borrowerName = member.Name
			}
		}
		fmt.Printf("%-5d %-30s %-25s %-10t %-25s\n", book.ID, book.Title, book.Author, book.Available, borrowerName)
	}
}

func handleCheckout(sc *bufio.Scanner, mgr *library.LibraryManager) {
	fmt.Print("Book ID: ")
	if !sc.Scan() {
		return
	}
	bookIDStr := strings.TrimSpace(sc.Text())
	bookID, err := strconv.ParseInt(bookIDStr, 10, 64)
	if err != nil {
		fmt.Printf("Invalid book ID: %s\n", bookIDStr)
		return
	}

	fmt.Print("Member ID: ")
	if !sc.Scan() {
		return
	}
	memberIDStr := strings.TrimSpace(sc.Text())
	memberID, err := strconv.ParseInt(memberIDStr, 10, 64)
	if err != nil {
		fmt.Printf("Invalid member ID: %s\n", memberIDStr)
		return
	}

	// Authenticate the member
	if err := authenticateUser(sc, mgr, memberID); err != nil {
		fmt.Printf("Authentication failed: %v\n", err)
		return
	}

	if err := mgr.CheckoutBook(bookID, memberID); err != nil {
		fmt.Printf("Error checking out book: %v\n", err)
		return
	}

	// Get member and book info for confirmation
	member, _ := mgr.GetMember(memberID)
	book, _ := mgr.GetBook(bookID)
	fmt.Printf("Book '%s' checked out to %s\n", book.Title, member.Name)
}

func handleReturn(sc *bufio.Scanner, mgr *library.LibraryManager) {
	fmt.Print("Book ID: ")
	if !sc.Scan() {
		return
	}
	bookIDStr := strings.TrimSpace(sc.Text())
	bookID, err := strconv.ParseInt(bookIDStr, 10, 64)
	if err != nil {
		fmt.Printf("Invalid book ID: %s\n", bookIDStr)
		return
	}

	fmt.Print("Member ID: ")
	if !sc.Scan() {
		return
	}
	memberIDStr := strings.TrimSpace(sc.Text())
	memberID, err := strconv.ParseInt(memberIDStr, 10, 64)
	if err != nil {
		fmt.Printf("Invalid member ID: %s\n", memberIDStr)
		return
	}

	// Authenticate the member
	if err := authenticateUser(sc, mgr, memberID); err != nil {
		fmt.Printf("Authentication failed: %v\n", err)
		return
	}

	returnedBy, assignedTo, err := mgr.ReturnBookWithDetails(bookID, memberID)
	if err != nil {
		fmt.Printf("Error returning book: %v\n", err)
		return
	}

	// Get book info
	book, _ := mgr.GetBook(bookID)
	returnedMember, _ := mgr.GetMember(returnedBy)

	fmt.Printf("Book '%s' returned by %s\n", book.Title, returnedMember.Name)

	if assignedTo > 0 {
		assignedMember, _ := mgr.GetMember(assignedTo)
		fmt.Printf("Book automatically assigned to %s (next in reservation queue)\n", assignedMember.Name)
	} else {
		fmt.Println("Book is now available for checkout")
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
		fmt.Printf("Invalid book ID: %s\n", bookIDStr)
		return
	}

	fmt.Print("Member ID: ")
	if !sc.Scan() {
		return
	}
	memberIDStr := strings.TrimSpace(sc.Text())
	memberID, err := strconv.ParseInt(memberIDStr, 10, 64)
	if err != nil {
		fmt.Printf("Invalid member ID: %s\n", memberIDStr)
		return
	}

	// Authenticate the member
	if err := authenticateUser(sc, mgr, memberID); err != nil {
		fmt.Printf("Authentication failed: %v\n", err)
		return
	}

	err = mgr.ReserveBook(bookID, memberID)
	if err != nil {
		fmt.Printf("Error reserving book: %v\n", err)
		return
	}

	// Get member and book info for confirmation
	member, _ := mgr.GetMember(memberID)
	book, _ := mgr.GetBook(bookID)

	if book.Available {
		fmt.Printf("Book '%s' immediately checked out to %s\n", book.Title, member.Name)
	} else {
		fmt.Printf("Book '%s' reserved for %s\n", book.Title, member.Name)

		// Show current position in queue
		reservations, err := mgr.GetReservations(bookID)
		if err == nil {
			for i, reservedMember := range reservations {
				if reservedMember.ID == memberID {
					fmt.Printf("Position in queue: %d\n", i+1)
					break
				}
			}
		}
	}
}

func handleListReservations(sc *bufio.Scanner, mgr *library.LibraryManager) {
	fmt.Print("Book ID (or press Enter for all books): ")
	if !sc.Scan() {
		return
	}
	bookIDStr := strings.TrimSpace(sc.Text())

	// If no Book ID provided, show reservations for all books
	if bookIDStr == "" {
		handleListAllReservations(mgr)
		return
	}

	bookID, err := strconv.ParseInt(bookIDStr, 10, 64)
	if err != nil {
		fmt.Printf("Invalid book ID: %s\n", bookIDStr)
		return
	}

	book, err := mgr.GetBook(bookID)
	if err != nil {
		fmt.Printf("Error: Book with ID %d not found\n", bookID)
		return
	}

	reservations, err := mgr.GetReservations(bookID)
	if err != nil {
		fmt.Printf("Error retrieving reservations: %v\n", err)
		return
	}

	fmt.Printf("Reservations for '%s' by %s:\n", book.Title, book.Author)

	if len(reservations) == 0 {
		fmt.Println("No reservations for this book.")
		return
	}

	fmt.Printf("%-10s %-5s %-30s\n", "Position", "ID", "Name")
	fmt.Println(strings.Repeat("-", 50))

	for i, member := range reservations {
		fmt.Printf("%-10d %-5d %-30s\n", i+1, member.ID, member.Name)
	}
}

func handleListAllReservations(mgr *library.LibraryManager) {
	books, err := mgr.GetAllBooks()
	if err != nil {
		fmt.Printf("Error retrieving books: %v\n", err)
		return
	}

	if len(books) == 0 {
		fmt.Println("No books in the library.")
		return
	}

	fmt.Println("Reservation Status for All Books:")
	fmt.Printf("%-5s %-30s %-25s %-12s %-30s %s\n", "ID", "Title", "Author", "Status", "Current Borrower", "Reservations")
	fmt.Println(strings.Repeat("-", 130))

	hasAnyReservations := false

	for _, book := range books {
		// Get current borrower info
		var statusInfo, borrowerInfo string
		if book.Available {
			statusInfo = "Available"
			borrowerInfo = "None"
		} else {
			statusInfo = "Checked Out"
			if member, err := mgr.GetMember(book.BorrowerID); err == nil {
				borrowerInfo = fmt.Sprintf("%s (ID: %d)", member.Name, member.ID)
			} else {
				borrowerInfo = fmt.Sprintf("ID: %d", book.BorrowerID)
			}
		}

		// Get reservations for this book
		reservations, err := mgr.GetReservations(book.ID)
		var reservationInfo string
		if err != nil || len(reservations) == 0 {
			reservationInfo = "None"
		} else {
			hasAnyReservations = true
			var queueList []string
			for i, member := range reservations {
				queueList = append(queueList, fmt.Sprintf("%d.%s(ID:%d)", i+1, member.Name, member.ID))
			}
			reservationInfo = strings.Join(queueList, ", ")
		}

		fmt.Printf("%-5d %-30s %-25s %-12s %-30s %s\n",
			book.ID,
			truncateString(book.Title, 30),
			truncateString(book.Author, 25),
			statusInfo,
			truncateString(borrowerInfo, 30),
			reservationInfo)
	}

	if !hasAnyReservations {
		fmt.Println("\nNo active reservations in the system.")
	} else {
		fmt.Printf("\nTotal books: %d | Books with reservations: ", len(books))
		reservedCount := 0
		for _, book := range books {
			if reservations, err := mgr.GetReservations(book.ID); err == nil && len(reservations) > 0 {
				reservedCount++
			}
		}
		fmt.Printf("%d\n", reservedCount)
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
		fmt.Printf("Invalid book ID: %s\n", bookIDStr)
		return
	}

	fmt.Print("Member ID: ")
	if !sc.Scan() {
		return
	}
	memberIDStr := strings.TrimSpace(sc.Text())
	memberID, err := strconv.ParseInt(memberIDStr, 10, 64)
	if err != nil {
		fmt.Printf("Invalid member ID: %s\n", memberIDStr)
		return
	}

	// Authenticate the member
	if err := authenticateUser(sc, mgr, memberID); err != nil {
		fmt.Printf("Authentication failed: %v\n", err)
		return
	}

	if err := mgr.CancelReservation(bookID, memberID); err != nil {
		fmt.Printf("Error cancelling reservation: %v\n", err)
		return
	}

	// Get member and book info for confirmation
	member, _ := mgr.GetMember(memberID)
	book, _ := mgr.GetBook(bookID)
	fmt.Printf("Reservation for '%s' cancelled for %s\n", book.Title, member.Name)
}

func handleUpdateContent(sc *bufio.Scanner, mgr *library.LibraryManager) {
	fmt.Print("Book ID: ")
	if !sc.Scan() {
		return
	}
	bookIDStr := strings.TrimSpace(sc.Text())
	bookID, err := strconv.ParseInt(bookIDStr, 10, 64)
	if err != nil {
		fmt.Printf("Invalid book ID: %s\n", bookIDStr)
		return
	}

	fmt.Print("Path to text file: ")
	if !sc.Scan() {
		return
	}
	path := strings.TrimSpace(sc.Text())

	if err := mgr.UpdateBookContentFromFile(bookID, path); err != nil {
		fmt.Printf("Error updating book content: %v\n", err)
		return
	}

	book, _ := mgr.GetBook(bookID)
	fmt.Printf("Content updated for book '%s'\n", book.Title)
}

func handleReadBook(sc *bufio.Scanner, mgr *library.LibraryManager) {
	fmt.Print("Book ID: ")
	if !sc.Scan() {
		return
	}
	bookIDStr := strings.TrimSpace(sc.Text())
	bookID, err := strconv.ParseInt(bookIDStr, 10, 64)
	if err != nil {
		fmt.Printf("Invalid book ID: %s\n", bookIDStr)
		return
	}

	fmt.Print("Member ID: ")
	if !sc.Scan() {
		return
	}
	memberIDStr := strings.TrimSpace(sc.Text())
	memberID, err := strconv.ParseInt(memberIDStr, 10, 64)
	if err != nil {
		fmt.Printf("Invalid member ID: %s\n", memberIDStr)
		return
	}

	// Authenticate the member
	if err := authenticateUser(sc, mgr, memberID); err != nil {
		fmt.Printf("Authentication failed: %v\n", err)
		return
	}

	if err := mgr.ReadBook(bookID, memberID); err != nil {
		fmt.Printf("Error reading book: %v\n", err)
		return
	}
}

func truncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength-3] + "..."
}

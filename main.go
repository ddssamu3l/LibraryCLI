package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"library-management/library"
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
	fmt.Println("Available commands: add book, add member, list books, list members, search book, advanced search, checkout, return, reserve, list reservations, cancel reservation, update content, read book, exit")

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

	id, err := mgr.AddMember(name)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Added member with ID %d\n", id)
	}
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
	for _, m := range members {
		fmt.Printf("%d: %s\n", m.ID, m.Name)
	}
}

func handleSearchBooks(sc *bufio.Scanner, mgr *library.LibraryManager) {
	fmt.Print("Query: ")
	if !sc.Scan() {
		return
	}
	q := strings.TrimSpace(sc.Text())
	results, err := mgr.SearchBooks(q)
	if err != nil {
		fmt.Printf("Search error: %v\n", err)
		return
	}
	fmt.Printf("Found %d result(s)\n", len(results))
	for _, b := range results {
		fmt.Printf("%d: %s by %s\n", b.ID, b.Title, b.Author)
	}
}

func handleCheckout(sc *bufio.Scanner, mgr *library.LibraryManager) {
	fmt.Print("Book ID: ")
	if !sc.Scan() {
		return
	}
	bookIDStr := strings.TrimSpace(sc.Text())
	bookID, _ := strconv.ParseInt(bookIDStr, 10, 64)

	fmt.Print("Member ID: ")
	if !sc.Scan() {
		return
	}
	memIDStr := strings.TrimSpace(sc.Text())
	memberID, _ := strconv.ParseInt(memIDStr, 10, 64)

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
	bookID, _ := strconv.ParseInt(bookIDStr, 10, 64)

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

	fmt.Print("Member ID: ")
	if !sc.Scan() {
		return
	}
	memIDStr := strings.TrimSpace(sc.Text())
	memberID, err := strconv.ParseInt(memIDStr, 10, 64)
	if err != nil {
		fmt.Println("Invalid member ID.")
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
	fmt.Println("1. List reservations for a book")
	fmt.Println("2. List reservations for a member")
	fmt.Print("Choose option (1-2): ")

	if !sc.Scan() {
		return
	}
	option := strings.TrimSpace(sc.Text())

	switch option {
	case "1":
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
		fmt.Print("Member ID: ")
		if !sc.Scan() {
			return
		}
		memIDStr := strings.TrimSpace(sc.Text())
		memberID, err := strconv.ParseInt(memIDStr, 10, 64)
		if err != nil {
			fmt.Println("Invalid member ID.")
			return
		}

		reservations, err := mgr.GetMemberReservations(memberID)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		if len(reservations) == 0 {
			fmt.Println("No active reservations for this member.")
			return
		}

		fmt.Printf("Active reservations for member %d:\n", memberID)
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

	fmt.Print("Member ID: ")
	if !sc.Scan() {
		return
	}
	memIDStr := strings.TrimSpace(sc.Text())
	memberID, err := strconv.ParseInt(memIDStr, 10, 64)
	if err != nil {
		fmt.Println("Invalid member ID.")
		return
	}

	if err := mgr.CancelReservation(bookID, memberID); err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Println("Reservation cancelled successfully.")
	}
}

func handleUpdateContent(sc *bufio.Scanner, mgr *library.LibraryManager) {
	fmt.Print("Book ID: ")
	if !sc.Scan() {
		return
	}
	idStr := strings.TrimSpace(sc.Text())
	bookID, errConv := strconv.ParseInt(idStr, 10, 64)
	if errConv != nil {
		fmt.Println("Invalid book ID.")
		return
	}

	fmt.Print("Path to text file: ")
	if !sc.Scan() {
		return
	}
	path := strings.TrimSpace(sc.Text())

	if err := mgr.UpdateBookContentFromFile(bookID, path); err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Println("Content updated.")
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

	if err := mgr.ReadBook(bookID, memberID); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

func truncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	if maxLength <= 3 {
		return s[:maxLength]
	}
	return s[:maxLength-3] + "..."
}

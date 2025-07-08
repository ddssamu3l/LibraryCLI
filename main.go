package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"library-management/library"
)

var (
	dbFile    = "library.db"
	lm        *library.LibraryManager
	isVerbose bool
)

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
func authenticateUser(memberID int64) error {
	password, err := readPassword("Enter your password: ")
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}

	if err := lm.AuthenticateMember(memberID, password); err != nil {
		return err
	}

	return nil
}

// parseAndAuthenticateMember parses member ID and authenticates them
func parseAndAuthenticateMember(memberIDStr string) (int64, error) {
	memberID, err := strconv.ParseInt(memberIDStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid member ID: %s", memberIDStr)
	}

	if err := authenticateUser(memberID); err != nil {
		return 0, err
	}

	return memberID, nil
}

func main() {
	var err error
	lm, err = library.NewLibraryManager(dbFile)
	if err != nil {
		log.Fatalf("Failed to open library database: %v", err)
	}
	defer lm.Close()

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

var rootCmd = &cobra.Command{
	Use:   "library-management",
	Short: "A CLI tool for managing a library with secure authentication",
	Long: `A comprehensive library management system with book cataloging, 
member management, circulation, reservations, and secure authentication.`,
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().BoolVarP(&isVerbose, "verbose", "v", false, "Enable verbose output")

	// Book management commands (no authentication required)
	rootCmd.AddCommand(&cobra.Command{
		Use:   "add-book [title] [author]",
		Short: "Add a new book to the library",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			title, author := args[0], args[1]
			id, err := lm.AddBook(title, author)
			if err != nil {
				fmt.Printf("Error adding book: %v\n", err)
				return
			}
			fmt.Printf("Book added with ID: %d\n", id)
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "add-book-from-file [title] [author] [file-path]",
		Short: "Add a book from a text file",
		Args:  cobra.ExactArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			title, author, path := args[0], args[1], args[2]
			id, err := lm.AddBookFromFile(title, author, path)
			if err != nil {
				fmt.Printf("Error adding book from file: %v\n", err)
				return
			}
			fmt.Printf("Book added from file with ID: %d\n", id)
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "list-books",
		Short: "List all books in the library",
		Run: func(cmd *cobra.Command, args []string) {
			books, err := lm.GetAllBooks()
			if err != nil {
				fmt.Printf("Error retrieving books: %v\n", err)
				return
			}

			if len(books) == 0 {
				fmt.Println("No books in the library.")
				return
			}

			fmt.Printf("%-5s %-30s %-25s %-10s %-25s\n", "ID", "Title", "Author", "Available", "Borrower")
			fmt.Println(strings.Repeat("-", 100))

			for _, book := range books {
				borrowerName := ""
				if !book.Available && book.BorrowerID > 0 {
					if member, err := lm.GetMember(book.BorrowerID); err == nil {
						borrowerName = member.Name
					}
				}
				fmt.Println(library.PrettyBook(book, borrowerName))
			}
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "search-books [query]",
		Short: "Search for books by title or author",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			query := args[0]
			books, err := lm.SearchBooks(query)
			if err != nil {
				fmt.Printf("Error searching books: %v\n", err)
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
					if member, err := lm.GetMember(book.BorrowerID); err == nil {
						borrowerName = member.Name
					}
				}
				fmt.Println(library.PrettyBook(book, borrowerName))
			}
		},
	})

	// Member management commands
	rootCmd.AddCommand(&cobra.Command{
		Use:   "add-member [name]",
		Short: "Add a new member with password",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]

			password, err := readPassword(fmt.Sprintf("Enter password for %s: ", name))
			if err != nil {
				fmt.Printf("Error reading password: %v\n", err)
				return
			}

			if strings.TrimSpace(password) == "" {
				fmt.Println("Error: Password cannot be empty")
				return
			}

			id, err := lm.AddMember(name, password)
			if err != nil {
				fmt.Printf("Error adding member: %v\n", err)
				return
			}
			fmt.Printf("Member '%s' added with ID: %d\n", name, id)
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "reset-password [member-id]",
		Short: "Reset a member's password",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			memberID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				fmt.Printf("Invalid member ID: %s\n", args[0])
				return
			}

			// Verify member exists and get their name
			member, err := lm.GetMember(memberID)
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

			if err := lm.ResetMemberPassword(memberID, newPassword); err != nil {
				fmt.Printf("Error resetting password: %v\n", err)
				return
			}

			fmt.Printf("Password successfully reset for %s (ID: %d)\n", member.Name, memberID)
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "list-members",
		Short: "List all library members",
		Run: func(cmd *cobra.Command, args []string) {
			members, err := lm.GetAllMembers()
			if err != nil {
				fmt.Printf("Error retrieving members: %v\n", err)
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
		},
	})

	// Protected circulation commands (require authentication)
	rootCmd.AddCommand(&cobra.Command{
		Use:   "checkout [book-id] [member-id]",
		Short: "Check out a book to a member (requires authentication)",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			bookID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				fmt.Printf("Invalid book ID: %s\n", args[0])
				return
			}

			memberID, err := parseAndAuthenticateMember(args[1])
			if err != nil {
				fmt.Printf("Authentication failed: %v\n", err)
				return
			}

			if err := lm.CheckoutBook(bookID, memberID); err != nil {
				fmt.Printf("Error checking out book: %v\n", err)
				return
			}

			// Get member and book info for confirmation
			member, _ := lm.GetMember(memberID)
			book, _ := lm.GetBook(bookID)
			fmt.Printf("Book '%s' checked out to %s\n", book.Title, member.Name)
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "return [book-id] [member-id]",
		Short: "Return a book (requires authentication)",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			bookID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				fmt.Printf("Invalid book ID: %s\n", args[0])
				return
			}

			memberID, err := parseAndAuthenticateMember(args[1])
			if err != nil {
				fmt.Printf("Authentication failed: %v\n", err)
				return
			}

			returnedBy, assignedTo, err := lm.ReturnBookWithDetails(bookID, memberID)
			if err != nil {
				fmt.Printf("Error returning book: %v\n", err)
				return
			}

			// Get book info
			book, _ := lm.GetBook(bookID)
			returnedMember, _ := lm.GetMember(returnedBy)

			fmt.Printf("Book '%s' returned by %s\n", book.Title, returnedMember.Name)

			if assignedTo > 0 {
				assignedMember, _ := lm.GetMember(assignedTo)
				fmt.Printf("Book automatically assigned to %s (next in reservation queue)\n", assignedMember.Name)
			} else {
				fmt.Println("Book is now available for checkout")
			}
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "reserve [book-id] [member-id]",
		Short: "Reserve a book (requires authentication)",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			bookID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				fmt.Printf("Invalid book ID: %s\n", args[0])
				return
			}

			memberID, err := parseAndAuthenticateMember(args[1])
			if err != nil {
				fmt.Printf("Authentication failed: %v\n", err)
				return
			}

			err = lm.ReserveBook(bookID, memberID)
			if err != nil {
				fmt.Printf("Error reserving book: %v\n", err)
				return
			}

			// Get member and book info for confirmation
			member, _ := lm.GetMember(memberID)
			book, _ := lm.GetBook(bookID)

			if book.Available {
				fmt.Printf("Book '%s' immediately checked out to %s\n", book.Title, member.Name)
			} else {
				fmt.Printf("Book '%s' reserved for %s\n", book.Title, member.Name)

				// Show current position in queue
				reservations, err := lm.GetReservations(bookID)
				if err == nil {
					for i, reservedMember := range reservations {
						if reservedMember.ID == memberID {
							fmt.Printf("Position in queue: %d\n", i+1)
							break
						}
					}
				}
			}
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "cancel-reservation [book-id] [member-id]",
		Short: "Cancel a book reservation (requires authentication)",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			bookID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				fmt.Printf("Invalid book ID: %s\n", args[0])
				return
			}

			memberID, err := parseAndAuthenticateMember(args[1])
			if err != nil {
				fmt.Printf("Authentication failed: %v\n", err)
				return
			}

			if err := lm.CancelReservation(bookID, memberID); err != nil {
				fmt.Printf("Error cancelling reservation: %v\n", err)
				return
			}

			// Get member and book info for confirmation
			member, _ := lm.GetMember(memberID)
			book, _ := lm.GetBook(bookID)
			fmt.Printf("Reservation for '%s' cancelled for %s\n", book.Title, member.Name)
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "read-book [book-id] [member-id]",
		Short: "Read a book with pagination (requires authentication)",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			bookID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				fmt.Printf("Invalid book ID: %s\n", args[0])
				return
			}

			memberID, err := parseAndAuthenticateMember(args[1])
			if err != nil {
				fmt.Printf("Authentication failed: %v\n", err)
				return
			}

			if err := lm.ReadBook(bookID, memberID); err != nil {
				fmt.Printf("Error reading book: %v\n", err)
				return
			}
		},
	})

	// Informational commands (no authentication required)
	rootCmd.AddCommand(&cobra.Command{
		Use:   "list-reservations [book-id]",
		Short: "List all reservations for a book",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			bookID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				fmt.Printf("Invalid book ID: %s\n", args[0])
				return
			}

			book, err := lm.GetBook(bookID)
			if err != nil {
				fmt.Printf("Error: Book with ID %d not found\n", bookID)
				return
			}

			reservations, err := lm.GetReservations(bookID)
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
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "member-reservations [member-id]",
		Short: "List all reservations for a member (requires authentication)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			memberID, err := parseAndAuthenticateMember(args[0])
			if err != nil {
				fmt.Printf("Authentication failed: %v\n", err)
				return
			}

			member, _ := lm.GetMember(memberID)
			reservations, err := lm.GetMemberReservations(memberID)
			if err != nil {
				fmt.Printf("Error retrieving reservations: %v\n", err)
				return
			}

			fmt.Printf("Reservations for %s (ID: %d):\n", member.Name, memberID)

			if len(reservations) == 0 {
				fmt.Println("No active reservations.")
				return
			}

			fmt.Printf("%-5s %-30s %-25s\n", "ID", "Title", "Author")
			fmt.Println(strings.Repeat("-", 65))

			for _, book := range reservations {
				fmt.Printf("%-5d %-30s %-25s\n", book.ID, book.Title, book.Author)
			}
		},
	})

	// Administrative/utility commands
	rootCmd.AddCommand(&cobra.Command{
		Use:   "update-book-content [book-id] [file-path]",
		Short: "Update a book's content from a file",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			bookID, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				fmt.Printf("Invalid book ID: %s\n", args[0])
				return
			}

			filePath := args[1]
			if !filepath.IsAbs(filePath) {
				filePath = filepath.Join(".", filePath)
			}

			if err := lm.UpdateBookContentFromFile(bookID, filePath); err != nil {
				fmt.Printf("Error updating book content: %v\n", err)
				return
			}

			book, _ := lm.GetBook(bookID)
			fmt.Printf("Content updated for book '%s'\n", book.Title)
		},
	})

	// Import command for bulk operations
	rootCmd.AddCommand(&cobra.Command{
		Use:   "import-books [directory]",
		Short: "Import books from text files in a directory",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			directory := args[0]

			files, err := os.ReadDir(directory)
			if err != nil {
				fmt.Printf("Error reading directory: %v\n", err)
				return
			}

			imported := 0
			for _, file := range files {
				if file.IsDir() || !strings.HasSuffix(file.Name(), ".txt") {
					continue
				}

				fileName := strings.TrimSuffix(file.Name(), ".txt")
				parts := strings.SplitN(fileName, " - ", 2)
				if len(parts) != 2 {
					fmt.Printf("Skipping file %s (expected format: 'Title - Author.txt')\n", file.Name())
					continue
				}

				title, author := parts[0], parts[1]
				filePath := filepath.Join(directory, file.Name())

				bookID, err := lm.AddBookFromFile(title, author, filePath)
				if err != nil {
					fmt.Printf("Error importing %s: %v\n", file.Name(), err)
					continue
				}

				if isVerbose {
					fmt.Printf("Imported: %s by %s (ID: %d)\n", title, author, bookID)
				}
				imported++
			}

			fmt.Printf("Successfully imported %d books from %s\n", imported, directory)
		},
	})
}

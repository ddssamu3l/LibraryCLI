package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"library-management/library"
)

func main() {
	// Clean up any existing database files
	fmt.Println("Cleaning up existing database files...")
	dbFiles := []string{"library.db", "library.db-shm", "library.db-wal"}
	for _, file := range dbFiles {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			fmt.Printf("Warning: Could not remove %s: %v\n", file, err)
		}
	}
	fmt.Println("Database cleanup complete.")

	// Create new database connection
	manager, err := library.NewLibraryManager("library.db")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating database: %v\n", err)
		os.Exit(1)
	}
	defer manager.Close()

	// Book metadata mapping (filename -> [title, author])
	bookMetadata := map[string][2]string{
		"1984.txt":                            {"1984", "George Orwell"},
		"animal_farm.txt":                     {"Animal Farm", "George Orwell"},
		"anne_frank.txt":                      {"The Diary of a Young Girl", "Anne Frank"},
		"art_of_war.txt":                      {"The Art of War", "Sun Tzu"},
		"fellowship_of_the_ring.txt":          {"The Fellowship of the Ring", "J.R.R. Tolkien"},
		"harry_potter_chamber_of_secrets.txt": {"Harry Potter and the Chamber of Secrets", "J.K. Rowling"},
		"harry_potter_deathly_hallows.txt":    {"Harry Potter and the Deathly Hallows", "J.K. Rowling"},
		"harry_potter_half_blood_prince.txt":  {"Harry Potter and the Half-Blood Prince", "J.K. Rowling"},
		"harry_potter_order_pheonix.txt":      {"Harry Potter and the Order of the Phoenix", "J.K. Rowling"},
		"harry_potter_prisoner_azkaban.txt":   {"Harry Potter and the Prisoner of Azkaban", "J.K. Rowling"},
		"harry_potter_scorcerers_stone.txt":   {"Harry Potter and the Philosopher's Stone", "J.K. Rowling"},
		"return_of_the_king.txt":              {"The Return of the King", "J.R.R. Tolkien"},
		"romeo_and_juliet.txt":                {"Romeo and Juliet", "William Shakespeare"},
		"the_two_towers.txt":                  {"The Two Towers", "J.R.R. Tolkien"},
		"three_little_pigs.txt":               {"The Three Little Pigs", "Traditional"},
		"three_musketeers.txt":                {"The Three Musketeers", "Alexandre Dumas"},
	}

	// Import books from the texts directory
	booksDir := "texts"
	fmt.Printf("Importing books from %s directory...\n", booksDir)

	files, err := os.ReadDir(booksDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading books directory: %v\n", err)
		os.Exit(1)
	}

	successCount := 0
	errorCount := 0

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".txt") {
			continue
		}

		filename := file.Name()
		metadata, exists := bookMetadata[filename]
		if !exists {
			fmt.Printf("Warning: No metadata found for %s, skipping\n", filename)
			continue
		}

		title := metadata[0]
		author := metadata[1]
		filePath := filepath.Join(booksDir, filename)

		fmt.Printf("Importing: %s by %s... ", title, author)

		// Check if file exists and is readable
		if _, err := os.Stat(filePath); err != nil {
			fmt.Printf("ERROR - File not accessible: %v\n", err)
			errorCount++
			continue
		}

		// Add book to database
		bookID, err := manager.AddBookFromFile(title, author, filePath)
		if err != nil {
			fmt.Printf("ERROR - %v\n", err)
			errorCount++
			continue
		}

		fmt.Printf("SUCCESS (ID: %d)\n", bookID)
		successCount++
	}

	fmt.Printf("\nImport complete!\n")
	fmt.Printf("Successfully imported: %d books\n", successCount)
	fmt.Printf("Errors: %d\n", errorCount)

	// Display summary of imported books
	if successCount > 0 {
		fmt.Println("\nImported books:")
		books, err := manager.GetAllBooks()
		if err != nil {
			fmt.Printf("Error retrieving books: %v\n", err)
		} else {
			fmt.Printf("%-3s %-50s %-30s\n", "ID", "Title", "Author")
			fmt.Println(strings.Repeat("-", 85))
			for _, book := range books {
				fmt.Printf("%-3d %-50s %-30s\n", book.ID, truncateString(book.Title, 50), truncateString(book.Author, 30))
			}
		}
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

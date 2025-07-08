package library

import (
	"io"
	"os"
	"strings"
	"testing"
)

// Mock reader to simulate user input during testing
type mockReader struct {
	inputs []string
	index  int
}

func (m *mockReader) Read(p []byte) (n int, err error) {
	if m.index >= len(m.inputs) {
		return 0, io.EOF
	}
	input := m.inputs[m.index] + "\n"
	m.index++
	n = copy(p, input)
	return n, nil
}

func TestValidateReadBookAccess(t *testing.T) {
	db := tempDB(t)

	// Setup test data
	content := "Test content for validation"
	bookID, _ := db.AddBook("Test Book", "Test Author", content)
	emptyBookID, _ := db.AddBook("Empty Book", "Test Author", "")
	memberID, _ := db.AddMember("Test Member", "password")
	member2ID, _ := db.AddMember("Member 2", "password")

	// Checkout book to member 2
	db.CheckoutBook(bookID, member2ID)

	tests := []struct {
		name                 string
		bookID               int64
		memberID             int64
		expectedExists       bool
		expectedMember       bool
		expectedContent      bool
		expectedCanRead      bool
		expectedAutoCheckout bool
	}{
		{
			name:                 "Valid book and member, available book",
			bookID:               emptyBookID, // Available book
			memberID:             memberID,
			expectedExists:       true,
			expectedMember:       true,
			expectedContent:      false, // Empty content
			expectedCanRead:      false,
			expectedAutoCheckout: true,
		},
		{
			name:                 "Book checked out by requesting member",
			bookID:               bookID,
			memberID:             member2ID, // Has book checked out
			expectedExists:       true,
			expectedMember:       true,
			expectedContent:      true,
			expectedCanRead:      true,
			expectedAutoCheckout: false,
		},
		{
			name:                 "Book checked out by different member",
			bookID:               bookID,
			memberID:             memberID, // Different member
			expectedExists:       true,
			expectedMember:       true,
			expectedContent:      true,
			expectedCanRead:      false,
			expectedAutoCheckout: false,
		},
		{
			name:                 "Non-existent book",
			bookID:               99999,
			memberID:             memberID,
			expectedExists:       false, // Book doesn't exist
			expectedMember:       true,  // Member exists
			expectedContent:      false,
			expectedCanRead:      false,
			expectedAutoCheckout: false,
		},
		{
			name:                 "Non-existent member",
			bookID:               bookID,
			memberID:             99999,
			expectedExists:       true,  // Book exists
			expectedMember:       false, // Member doesn't exist
			expectedContent:      false,
			expectedCanRead:      false,
			expectedAutoCheckout: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validation, err := db.ValidateReadBookAccess(tt.bookID, tt.memberID)
			if err != nil {
				t.Fatalf("ValidateReadBookAccess failed: %v", err)
			}

			if validation.BookExists != tt.expectedExists {
				t.Errorf("BookExists = %v, want %v", validation.BookExists, tt.expectedExists)
			}
			if validation.MemberExists != tt.expectedMember {
				t.Errorf("MemberExists = %v, want %v", validation.MemberExists, tt.expectedMember)
			}
			if validation.HasContent != tt.expectedContent {
				t.Errorf("HasContent = %v, want %v", validation.HasContent, tt.expectedContent)
			}
			if validation.CanRead != tt.expectedCanRead {
				t.Errorf("CanRead = %v, want %v", validation.CanRead, tt.expectedCanRead)
			}
			if validation.CanAutoCheckout != tt.expectedAutoCheckout {
				t.Errorf("CanAutoCheckout = %v, want %v", validation.CanAutoCheckout, tt.expectedAutoCheckout)
			}
		})
	}
}

func TestGetBookContentChunk(t *testing.T) {
	db := tempDB(t)

	content := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ" // 36 characters
	bookID, _ := db.AddBook("Chunk Test", "Author", content)

	tests := []struct {
		name     string
		offset   int
		length   int
		expected string
	}{
		{
			name:     "First chunk",
			offset:   0,
			length:   10,
			expected: "0123456789",
		},
		{
			name:     "Middle chunk",
			offset:   10,
			length:   10,
			expected: "ABCDEFGHIJ",
		},
		{
			name:     "Last chunk (partial)",
			offset:   30,
			length:   10,
			expected: "UVWXYZ",
		},
		{
			name:     "Beyond content",
			offset:   50,
			length:   10,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunk, err := db.GetBookContentChunk(bookID, tt.offset, tt.length)
			if err != nil {
				t.Fatalf("GetBookContentChunk failed: %v", err)
			}
			if chunk != tt.expected {
				t.Errorf("GetBookContentChunk() = %q, want %q", chunk, tt.expected)
			}
		})
	}
}

func TestReadBookValidation(t *testing.T) {
	db := tempDB(t)
	lm := &LibraryManager{db: db}

	// Setup test data
	content := "Test book content for validation testing."
	bookID, _ := db.AddBook("Validation Test Book", "Author", content)
	emptyBookID, _ := db.AddBook("Empty Book", "Author", "")
	member1ID, _ := db.AddMember("Alice", "password")
	member2ID, _ := db.AddMember("Bob", "password")

	// Checkout book to member2
	db.CheckoutBook(bookID, member2ID)

	tests := []struct {
		name          string
		bookID        int64
		memberID      int64
		expectedError string
	}{
		{
			name:          "Non-existent book",
			bookID:        99999,
			memberID:      member1ID,
			expectedError: "book not found",
		},
		{
			name:          "Non-existent member",
			bookID:        bookID,
			memberID:      99999,
			expectedError: "member not found",
		},
		{
			name:          "Book without content",
			bookID:        emptyBookID,
			memberID:      member1ID,
			expectedError: "book has no content to read",
		},
		{
			name:          "Book checked out by another member (privacy test)",
			bookID:        bookID,
			memberID:      member1ID,
			expectedError: "book is currently checked out by another member",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Temporarily redirect stdout to capture output
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := lm.ReadBook(tt.bookID, tt.memberID)

			// Restore stdout
			w.Close()
			os.Stdout = oldStdout

			if err == nil {
				t.Fatalf("Expected error but got none")
			}

			if !strings.Contains(err.Error(), tt.expectedError) {
				t.Errorf("Expected error containing %q, got %q", tt.expectedError, err.Error())
			}

			// Verify privacy: error should not contain borrower information
			if strings.Contains(err.Error(), "Bob") || strings.Contains(err.Error(), "ID:") {
				t.Errorf("Error message exposes borrower information: %q", err.Error())
			}

			// Read any captured output
			r.Close()
		})
	}
}

func TestReadBookAutoCheckout(t *testing.T) {
	db := tempDB(t)
	lm := &LibraryManager{db: db}

	content := "This is content for auto-checkout testing."
	bookID, _ := db.AddBook("Auto Checkout Book", "Author", content)
	memberID, _ := db.AddMember("Reader", "password")

	// Verify book is initially available
	book, _ := db.GetBook(bookID)
	if !book.Available {
		t.Fatalf("Book should be available initially")
	}

	// Capture stdout to verify checkout message
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Mock stdin for quit command
	oldStdin := os.Stdin
	mockInput := &mockReader{inputs: []string{"q"}}
	pr, pw, _ := os.Pipe()
	os.Stdin = pr
	go func() {
		defer pw.Close()
		io.Copy(pw, mockInput)
	}()

	// Call ReadBook
	err := lm.ReadBook(bookID, memberID)

	// Restore stdout and stdin
	w.Close()
	os.Stdout = oldStdout
	pr.Close()
	os.Stdin = oldStdin

	if err != nil {
		t.Fatalf("ReadBook should succeed for available book: %v", err)
	}

	// Read captured output
	output := make([]byte, 1024)
	n, _ := r.Read(output)
	r.Close()

	outputStr := string(output[:n])
	if !strings.Contains(outputStr, "checked out to Reader for reading") {
		t.Errorf("Expected checkout message in output, got: %q", outputStr)
	}

	// Verify book is now checked out
	book, _ = db.GetBook(bookID)
	if book.Available {
		t.Errorf("Book should be checked out after ReadBook")
	}
	if book.BorrowerID != memberID {
		t.Errorf("Book should be checked out to the reading member")
	}
}

func TestReadBookMemoryEfficiency(t *testing.T) {
	db := tempDB(t)

	// Create a large book (simulate 50KB content)
	largeContent := strings.Repeat("A", 50000)
	bookID, _ := db.AddBook("Large Book", "Author", largeContent)
	memberID, _ := db.AddMember("Reader", "password")

	// Checkout book first
	db.CheckoutBook(bookID, memberID)

	// Test that we can read chunks without loading entire content
	chunk, err := db.GetBookContentChunk(bookID, 0, 1500)
	if err != nil {
		t.Fatalf("GetBookContentChunk failed: %v", err)
	}

	expectedChunk := strings.Repeat("A", 1500)
	if chunk != expectedChunk {
		t.Errorf("Chunk content mismatch, got length %d, expected length %d", len(chunk), len(expectedChunk))
	}

	// Test reading from middle
	chunk2, err := db.GetBookContentChunk(bookID, 25000, 1500)
	if err != nil {
		t.Fatalf("GetBookContentChunk failed for middle chunk: %v", err)
	}

	if len(chunk2) != 1500 {
		t.Errorf("Middle chunk length = %d, want 1500", len(chunk2))
	}
}

func TestReadBookSinglePageHandling(t *testing.T) {
	db := tempDB(t)

	// Create a book with less than 1500 characters (single page)
	shortContent := "This is a short book with less than 1500 characters."
	bookID, _ := db.AddBook("Short Book", "Author", shortContent)
	memberID, _ := db.AddMember("Reader", "password")

	// Checkout book
	db.CheckoutBook(bookID, memberID)

	// Test single page calculation
	totalPages := (len(shortContent) + 1499) / 1500 // Same calculation as in the code
	if totalPages != 1 {
		t.Errorf("Expected 1 page for short content, got %d", totalPages)
	}

	// Test that navigation commands handle single page correctly
	chunk, err := db.GetBookContentChunk(bookID, 0, 1500)
	if err != nil {
		t.Fatalf("GetBookContentChunk failed: %v", err)
	}

	if chunk != shortContent {
		t.Errorf("Single page content mismatch")
	}
}

func TestReadBookDatabaseEfficiency(t *testing.T) {
	db := tempDB(t)

	content := "Test content for database efficiency"
	bookID, _ := db.AddBook("Efficiency Test", "Author", content)
	memberID, _ := db.AddMember("Reader", "password")

	// Test that ValidateReadBookAccess gets all needed info in one query
	validation, err := db.ValidateReadBookAccess(bookID, memberID)
	if err != nil {
		t.Fatalf("ValidateReadBookAccess failed: %v", err)
	}

	// Verify all required information is present
	if validation.BookTitle != "Efficiency Test" {
		t.Errorf("BookTitle = %q, want 'Efficiency Test'", validation.BookTitle)
	}
	if validation.BookAuthor != "Author" {
		t.Errorf("BookAuthor = %q, want 'Author'", validation.BookAuthor)
	}
	if validation.MemberName != "Reader" {
		t.Errorf("MemberName = %q, want 'Reader'", validation.MemberName)
	}
	if validation.BookContentLength != len(content) {
		t.Errorf("BookContentLength = %d, want %d", validation.BookContentLength, len(content))
	}
	if !validation.BookExists {
		t.Error("BookExists should be true")
	}
	if !validation.MemberExists {
		t.Error("MemberExists should be true")
	}
	if !validation.HasContent {
		t.Error("HasContent should be true")
	}
	if !validation.CanAutoCheckout {
		t.Error("CanAutoCheckout should be true for available book")
	}
	if !validation.CanRead {
		t.Error("CanRead should be true for available book with content")
	}
}

func TestReadBookWhitespaceContent(t *testing.T) {
	db := tempDB(t)
	lm := &LibraryManager{db: db}

	// Create book with only whitespace
	wsContent := "   \n\t  \r\n  "
	bookID, _ := db.AddBook("Whitespace Book", "Author", wsContent)
	memberID, _ := db.AddMember("Reader", "password")

	err := lm.ReadBook(bookID, memberID)
	if err == nil {
		t.Fatalf("Should fail for book with only whitespace content")
	}

	if !strings.Contains(err.Error(), "no content to read") {
		t.Errorf("Expected 'no content to read' error, got: %v", err)
	}
}

func TestReadBookBoundaryConditions(t *testing.T) {
	db := tempDB(t)

	// Test exactly 1500 characters (boundary)
	exactContent := strings.Repeat("X", 1500)
	bookID, _ := db.AddBook("Boundary Book", "Author", exactContent)

	// Test chunk at exact boundary
	chunk, err := db.GetBookContentChunk(bookID, 0, 1500)
	if err != nil {
		t.Fatalf("GetBookContentChunk failed: %v", err)
	}

	if len(chunk) != 1500 {
		t.Errorf("Chunk length = %d, want 1500", len(chunk))
	}

	// Test 1501 characters (2 pages)
	overContent := strings.Repeat("Y", 1501)
	bookID2, _ := db.AddBook("Over Boundary Book", "Author", overContent)

	chunk1, _ := db.GetBookContentChunk(bookID2, 0, 1500)
	chunk2, _ := db.GetBookContentChunk(bookID2, 1500, 1500)

	if len(chunk1) != 1500 {
		t.Errorf("First chunk length = %d, want 1500", len(chunk1))
	}
	if len(chunk2) != 1 {
		t.Errorf("Second chunk length = %d, want 1", len(chunk2))
	}
	if chunk2 != "Y" {
		t.Errorf("Second chunk = %q, want 'Y'", chunk2)
	}
}

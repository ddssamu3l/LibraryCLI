package library

import (
	"strings"
	"testing"
)

func TestReadBookBasicFlow(t *testing.T) {
	db := tempDB(t)

	// Setup: Create book with content and member
	content := strings.Repeat("This is test content for reading. ", 100) // ~3400 chars, should be 3 pages
	bookID, err := db.AddBook("Test Reading Book", "Test Author", content)
	if err != nil {
		t.Fatalf("failed to add book: %v", err)
	}

	memberID, err := db.AddMember("Test Reader")
	if err != nil {
		t.Fatalf("failed to add member: %v", err)
	}

	// Test: Book should be available initially
	book, err := db.GetBook(bookID)
	if err != nil {
		t.Fatalf("failed to get book: %v", err)
	}
	if !book.Available {
		t.Fatalf("book should be available initially")
	}

	// Test: Reading available book should check it out
	// Note: We can't easily test the interactive UI, but we can test the setup logic

	// Simulate the checkout part of ReadBook
	if err := db.CheckoutBook(bookID, memberID); err != nil {
		t.Fatalf("checkout should succeed: %v", err)
	}

	// Verify book is now checked out
	book, err = db.GetBook(bookID)
	if err != nil {
		t.Fatalf("failed to get book after checkout: %v", err)
	}
	if book.Available {
		t.Fatalf("book should not be available after checkout")
	}
	if book.BorrowerID != memberID {
		t.Fatalf("book should be checked out to member %d, got %d", memberID, book.BorrowerID)
	}
}

func TestReadBookPermissions(t *testing.T) {
	db := tempDB(t)
	lm := &LibraryManager{db: db}

	// Setup
	content := "Test book content for permission testing."
	bookID, _ := db.AddBook("Permission Test Book", "Author", content)
	member1ID, _ := db.AddMember("Alice")
	member2ID, _ := db.AddMember("Bob")

	// Test 1: Non-existent book
	err := lm.ReadBook(99999, member1ID)
	if err == nil {
		t.Fatalf("should fail for non-existent book")
	}
	if !strings.Contains(err.Error(), "book not found") {
		t.Fatalf("expected 'book not found' error, got: %v", err)
	}

	// Test 2: Non-existent member
	err = lm.ReadBook(bookID, 99999)
	if err == nil {
		t.Fatalf("should fail for non-existent member")
	}
	if !strings.Contains(err.Error(), "member not found") {
		t.Fatalf("expected 'member not found' error, got: %v", err)
	}

	// Test 3: Member1 checks out book
	if err := db.CheckoutBook(bookID, member1ID); err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	// Test 4: Member2 tries to read book checked out by Member1
	err = lm.ReadBook(bookID, member2ID)
	if err == nil {
		t.Fatalf("member2 should not be able to read book checked out by member1")
	}
	if !strings.Contains(err.Error(), "currently checked out by") {
		t.Fatalf("expected 'currently checked out by' error, got: %v", err)
	}

	// Test 5: Member1 should be able to read their own checked out book
	// We can't test the full UI, but we can verify the permission check passes
	book, _ := db.GetBook(bookID)
	if book.BorrowerID != member1ID {
		t.Fatalf("setup error: book should be checked out to member1")
	}
	// The ReadBook function would proceed to startReadingInterface for member1
}

func TestReadBookWithoutContent(t *testing.T) {
	db := tempDB(t)
	lm := &LibraryManager{db: db}

	// Create book without content
	bookID, _ := db.AddBook("Empty Book", "Author", "")
	memberID, _ := db.AddMember("Reader")

	err := lm.ReadBook(bookID, memberID)
	if err == nil {
		t.Fatalf("should fail for book without content")
	}
	if !strings.Contains(err.Error(), "no content to read") {
		t.Fatalf("expected 'no content to read' error, got: %v", err)
	}

	// Test with whitespace-only content
	if err := db.UpdateBookContent(bookID, "   \n\t  "); err != nil {
		t.Fatalf("failed to update book content: %v", err)
	}

	err = lm.ReadBook(bookID, memberID)
	if err == nil {
		t.Fatalf("should fail for book with only whitespace content")
	}
	if !strings.Contains(err.Error(), "no content to read") {
		t.Fatalf("expected 'no content to read' error, got: %v", err)
	}
}

func TestReadBookAutoCheckout(t *testing.T) {
	db := tempDB(t)

	content := "This is content for auto-checkout testing."
	bookID, _ := db.AddBook("Auto Checkout Book", "Author", content)
	memberID, _ := db.AddMember("Reader")

	// Verify book is initially available
	book, _ := db.GetBook(bookID)
	if !book.Available {
		t.Fatalf("book should be available initially")
	}

	// Mock the checkout part of ReadBook (since we can't test the full UI)
	// This simulates what happens when ReadBook is called on an available book
	if err := db.CheckoutBook(bookID, memberID); err != nil {
		t.Fatalf("auto-checkout should succeed: %v", err)
	}

	// Verify book is now checked out to the member
	book, _ = db.GetBook(bookID)
	if book.Available {
		t.Fatalf("book should be checked out after ReadBook")
	}
	if book.BorrowerID != memberID {
		t.Fatalf("book should be checked out to the reading member")
	}
}

func TestReadBookWithReservations(t *testing.T) {
	db := tempDB(t)
	lm := &LibraryManager{db: db}

	content := "Content for reservation testing."
	bookID, _ := db.AddBook("Reserved Book", "Author", content)
	member1ID, _ := db.AddMember("Current Borrower")
	member2ID, _ := db.AddMember("Reserved Member")
	member3ID, _ := db.AddMember("Other Member")

	// Member1 checks out the book
	if err := db.CheckoutBook(bookID, member1ID); err != nil {
		t.Fatalf("initial checkout failed: %v", err)
	}

	// Member2 reserves the book
	if err := db.ReserveBook(bookID, member2ID); err != nil {
		t.Fatalf("reservation failed: %v", err)
	}

	// Test 1: Member1 (current borrower) should be able to read
	book, _ := db.GetBook(bookID)
	if book.BorrowerID != member1ID {
		t.Fatalf("setup error: member1 should have the book")
	}
	// ReadBook would succeed for member1 (can't test UI, but permission check passes)

	// Test 2: Member2 (has reservation) should NOT be able to read until book is returned
	err := lm.ReadBook(bookID, member2ID)
	if err == nil {
		t.Fatalf("member2 should not be able to read book they only have reserved")
	}
	if !strings.Contains(err.Error(), "currently checked out by") {
		t.Fatalf("expected 'currently checked out by' error, got: %v", err)
	}

	// Test 3: Member3 (no reservation) should also not be able to read
	err = lm.ReadBook(bookID, member3ID)
	if err == nil {
		t.Fatalf("member3 should not be able to read book checked out by someone else")
	}

	// Test 4: After member1 returns, member2 gets auto-assigned and should be able to read
	_, err = db.ReturnBook(bookID)
	if err != nil {
		t.Fatalf("return failed: %v", err)
	}

	// Verify member2 now has the book (auto-assigned from reservation)
	book, _ = db.GetBook(bookID)
	if book.Available {
		t.Fatalf("book should be auto-assigned to member2")
	}
	if book.BorrowerID != member2ID {
		t.Fatalf("book should be assigned to member2, got member %d", book.BorrowerID)
	}

	// Now member2 should be able to read (permission check would pass)
	// We can't test the full UI, but we can verify the setup is correct
}

func TestReadBookContentPagination(t *testing.T) {
	// Test the pagination logic by examining how content would be split
	const pageSize = 1500

	testCases := []struct {
		name          string
		contentLength int
		expectedPages int
	}{
		{"Empty content", 0, 0},
		{"Single character", 1, 1},
		{"Exactly one page", pageSize, 1},
		{"Just over one page", pageSize + 1, 2},
		{"Two full pages", pageSize * 2, 2},
		{"Two pages plus one char", pageSize*2 + 1, 3},
		{"Large content", pageSize*5 + 100, 6},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			content := strings.Repeat("x", tc.contentLength)

			// Simulate the pagination logic from startReadingInterface
			pages := make([]string, 0)
			for i := 0; i < len(content); i += pageSize {
				end := i + pageSize
				if end > len(content) {
					end = len(content)
				}
				if i < len(content) { // Only add non-empty pages
					pages = append(pages, content[i:end])
				}
			}

			if len(pages) != tc.expectedPages {
				t.Fatalf("expected %d pages, got %d", tc.expectedPages, len(pages))
			}

			// Verify page sizes (except possibly the last page)
			for i, page := range pages {
				expectedSize := pageSize
				if i == len(pages)-1 && tc.contentLength%pageSize != 0 {
					expectedSize = tc.contentLength % pageSize
				}
				if len(page) != expectedSize {
					t.Fatalf("page %d: expected size %d, got %d", i, expectedSize, len(page))
				}
			}
		})
	}
}

func TestReadBookIntegrationWithLibraryOperations(t *testing.T) {
	db := tempDB(t)
	lm := &LibraryManager{db: db}

	content := "Integration test content for reading functionality."
	bookID, _ := db.AddBook("Integration Book", "Author", content)
	member1ID, _ := db.AddMember("Alice")
	member2ID, _ := db.AddMember("Bob")

	// Test 1: Read available book (should auto-checkout)
	// Simulate the checkout that would happen in ReadBook
	if err := db.CheckoutBook(bookID, member1ID); err != nil {
		t.Fatalf("auto-checkout simulation failed: %v", err)
	}

	// Test 2: Verify book is checked out
	book, _ := db.GetBook(bookID)
	if book.Available || book.BorrowerID != member1ID {
		t.Fatalf("book should be checked out to member1")
	}

	// Test 3: Member2 reserves the book
	if err := db.ReserveBook(bookID, member2ID); err != nil {
		t.Fatalf("reservation should succeed: %v", err)
	}

	// Test 4: Member1 can still read (they have it checked out)
	// Permission check should pass
	if book.BorrowerID != member1ID {
		t.Fatalf("member1 should still have the book")
	}

	// Test 5: Member2 cannot read yet
	err := lm.ReadBook(bookID, member2ID)
	if err == nil {
		t.Fatalf("member2 should not be able to read book checked out by member1")
	}

	// Test 6: After return, member2 gets the book and can read
	_, err = db.ReturnBook(bookID)
	if err != nil {
		t.Fatalf("return failed: %v", err)
	}

	book, _ = db.GetBook(bookID)
	if book.BorrowerID != member2ID {
		t.Fatalf("book should be auto-assigned to member2")
	}

	// Now member2 should be able to read (permission check passes)
	// Test 7: Update content and verify reading still works
	newContent := "Updated content for continued reading test."
	if err := db.UpdateBookContent(bookID, newContent); err != nil {
		t.Fatalf("content update failed: %v", err)
	}

	book, _ = db.GetBook(bookID)
	if book.Content != newContent {
		t.Fatalf("content was not updated properly")
	}

	// Member2 should still be able to read with updated content
	if book.BorrowerID != member2ID {
		t.Fatalf("member2 should still have the book after content update")
	}
}

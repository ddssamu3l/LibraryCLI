package library

import (
	"path/filepath"
	"strings"
	"testing"
)

func tempDB(t *testing.T) *Database {
	t.Helper()
	dir := t.TempDir()
	db, err := NewDatabase(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("new db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestLargeTextInsertAndSearch(t *testing.T) {
	db := tempDB(t)
	huge := strings.Repeat("lorem ipsum ", 50_000) // ~550 KB
	_, err := db.AddBook("Epic", "Homer", huge)
	if err != nil {
		t.Fatalf("add book: %v", err)
	}

	res, err := db.SearchBooks("Homer")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("want 1 result, got %d", len(res))
	}
}

func TestCheckoutFlow(t *testing.T) {
	db := tempDB(t)
	bookID, _ := db.AddBook("Book", "Author", "txt")
	memberID, _ := db.AddMember("Alice")

	if err := db.CheckoutBook(bookID, memberID); err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if _, err := db.ReturnBook(bookID); err != nil {
		t.Fatalf("return: %v", err)
	}
}

// TestReservationSystem covers common reservation scenarios.
func TestReservationSystem(t *testing.T) {
	db := tempDB(t)

	bookID, err := db.AddBook("Test Book", "Test Author", "Test content")
	if err != nil {
		t.Fatalf("add book: %v", err)
	}

	member1ID, err := db.AddMember("Alice")
	if err != nil {
		t.Fatalf("add member 1: %v", err)
	}

	member2ID, err := db.AddMember("Bob")
	if err != nil {
		t.Fatalf("add member 2: %v", err)
	}

	member3ID, err := db.AddMember("Charlie")
	if err != nil {
		t.Fatalf("add member 3: %v", err)
	}

	// Reserve when available – should checkout immediately
	if err := db.ReserveBook(bookID, member1ID); err != nil {
		t.Fatalf("reserve available: %v", err)
	}
	book, _ := db.GetBook(bookID)
	if book.Available || book.BorrowerID != member1ID {
		t.Fatalf("expected borrower %d", member1ID)
	}

	// Reserve two others
	if err := db.ReserveBook(bookID, member2ID); err != nil {
		t.Fatalf("reserve queue: %v", err)
	}
	if err := db.ReserveBook(bookID, member3ID); err != nil {
		t.Fatalf("reserve queue2: %v", err)
	}

	// Check queue order
	res, _ := db.GetReservations(bookID)
	if len(res) != 2 || res[0].ID != member2ID || res[1].ID != member3ID {
		t.Fatalf("queue order incorrect")
	}

	// Return – assign to member2
	retID, _ := db.ReturnBook(bookID)
	if retID != member1ID {
		t.Fatalf("wrong returned id")
	}
	book, _ = db.GetBook(bookID)
	if book.BorrowerID != member2ID {
		t.Fatalf("expected borrower %d", member2ID)
	}

	// Return again – assign to member3
	_, _ = db.ReturnBook(bookID)
	book, _ = db.GetBook(bookID)
	if book.BorrowerID != member3ID {
		t.Fatalf("expected borrower %d", member3ID)
	}

	// Final return – book available
	_, _ = db.ReturnBook(bookID)
	book, _ = db.GetBook(bookID)
	if !book.Available {
		t.Fatalf("book should be available")
	}
}

func TestReservationEdgeCases(t *testing.T) {
	db := tempDB(t)

	bookID, _ := db.AddBook("Edge Book", "Edge", "content")
	memberID, _ := db.AddMember("Alice")

	// Test Case 1: Member reserves a book they already have checked out
	// First, checkout the book
	if err := db.CheckoutBook(bookID, memberID); err != nil {
		t.Fatalf("checkout failed: %v", err)
	}
	// Try to reserve the same book they already have checked out
	err := db.ReserveBook(bookID, memberID)
	if err == nil {
		t.Fatalf("expected error when reserving book already checked out by same member")
	}
	if !strings.Contains(err.Error(), "you can't reserve this book because you have already checked it out") {
		t.Fatalf("expected specific error message, got: %v", err)
	}
	// Return the book to clean up
	_, _ = db.ReturnBook(bookID)

	// Test Case 2: Member reserves the same book twice (book not checked out by them)
	// First, let another member checkout the book so it's unavailable
	otherMemberID, _ := db.AddMember("Bob")
	if err := db.CheckoutBook(bookID, otherMemberID); err != nil {
		t.Fatalf("checkout by other member failed: %v", err)
	}
	// Alice reserves the unavailable book (should create reservation)
	if err := db.ReserveBook(bookID, memberID); err != nil {
		t.Fatalf("first reservation should succeed: %v", err)
	}
	// Alice tries to reserve the same book again (should fail)
	err = db.ReserveBook(bookID, memberID)
	if err == nil {
		t.Fatalf("expected duplicate reservation error")
	}
	if !strings.Contains(err.Error(), "you already have a reservation for this book") {
		t.Fatalf("expected specific error message, got: %v", err)
	}

	// Test Case 3: Non-existent book and member IDs
	err = db.ReserveBook(99999, memberID)
	if err == nil {
		t.Fatalf("expected non-existent book error")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected 'does not exist' in error message, got: %v", err)
	}
	err = db.ReserveBook(bookID, 99999)
	if err == nil {
		t.Fatalf("expected non-existent member error")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected 'does not exist' in error message, got: %v", err)
	}
}

func TestCancelReservation(t *testing.T) {
	db := tempDB(t)
	bookID, _ := db.AddBook("Cancel Book", "Auth", "c")
	mem1, _ := db.AddMember("Alice")
	mem2, _ := db.AddMember("Bob")

	db.CheckoutBook(bookID, mem1)
	db.ReserveBook(bookID, mem2)

	if err := db.CancelReservation(bookID, mem2); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if err := db.CancelReservation(bookID, mem2); err == nil {
		t.Fatalf("expected error cancelling non-existent reservation")
	}
}

func TestGetMemberReservations(t *testing.T) {
	db := tempDB(t)
	b1, _ := db.AddBook("B1", "A1", "c")
	b2, _ := db.AddBook("B2", "A2", "c")
	mem1, _ := db.AddMember("Alice")
	mem2, _ := db.AddMember("Bob")

	db.CheckoutBook(b1, mem2)
	db.CheckoutBook(b2, mem2)
	db.ReserveBook(b1, mem1)
	db.ReserveBook(b2, mem1)

	books, _ := db.GetMemberReservations(mem1)
	if len(books) != 2 {
		t.Fatalf("want 2 reservations, got %d", len(books))
	}
}

// TestConcurrentReservations tests that concurrent reservation operations are handled safely.
func TestConcurrentReservations(t *testing.T) {
	db := tempDB(t)
	bookID, _ := db.AddBook("Concurrent Book", "Author", "content")
	member1ID, _ := db.AddMember("Alice")
	member2ID, _ := db.AddMember("Bob")

	// First member checks out the book
	if err := db.CheckoutBook(bookID, member1ID); err != nil {
		t.Fatalf("initial checkout failed: %v", err)
	}

	// Both members try to reserve simultaneously
	// This tests that our transaction handling prevents race conditions
	done1 := make(chan error, 1)
	done2 := make(chan error, 1)

	go func() {
		done1 <- db.ReserveBook(bookID, member1ID) // Should fail (already has book)
	}()

	go func() {
		done2 <- db.ReserveBook(bookID, member2ID) // Should succeed
	}()

	err1 := <-done1
	err2 := <-done2

	// One should succeed, one should fail
	if err1 == nil {
		t.Fatalf("member1 should not be able to reserve book they already have")
	}
	if err2 != nil {
		t.Fatalf("member2 reservation should succeed: %v", err2)
	}

	// Verify queue state
	reservations, _ := db.GetReservations(bookID)
	if len(reservations) != 1 || reservations[0].ID != member2ID {
		t.Fatalf("expected exactly one reservation for member2")
	}
}

// TestCompleteReservationWorkflow tests the entire reservation system end-to-end.
func TestCompleteReservationWorkflow(t *testing.T) {
	db := tempDB(t)

	// Setup: Create book and multiple members
	bookID, _ := db.AddBook("Popular Book", "Famous Author", "Great content")
	alice, _ := db.AddMember("Alice")
	bob, _ := db.AddMember("Bob")
	charlie, _ := db.AddMember("Charlie")
	diana, _ := db.AddMember("Diana")

	// Step 1: Alice reserves available book (immediate checkout)
	if err := db.ReserveBook(bookID, alice); err != nil {
		t.Fatalf("Alice's reservation failed: %v", err)
	}
	book, _ := db.GetBook(bookID)
	if book.Available || book.BorrowerID != alice {
		t.Fatalf("Alice should have the book checked out")
	}

	// Step 2: Bob, Charlie, and Diana reserve the unavailable book (queue up)
	if err := db.ReserveBook(bookID, bob); err != nil {
		t.Fatalf("Bob's reservation failed: %v", err)
	}
	if err := db.ReserveBook(bookID, charlie); err != nil {
		t.Fatalf("Charlie's reservation failed: %v", err)
	}
	if err := db.ReserveBook(bookID, diana); err != nil {
		t.Fatalf("Diana's reservation failed: %v", err)
	}

	// Verify queue order
	reservations, _ := db.GetReservations(bookID)
	if len(reservations) != 3 {
		t.Fatalf("expected 3 reservations, got %d", len(reservations))
	}
	expectedOrder := []int64{bob, charlie, diana}
	for i, expected := range expectedOrder {
		if reservations[i].ID != expected {
			t.Fatalf("queue position %d: expected member %d, got %d", i, expected, reservations[i].ID)
		}
	}

	// Step 3: Test edge cases
	// Alice tries to reserve again (should fail - already has book)
	if err := db.ReserveBook(bookID, alice); err == nil {
		t.Fatalf("Alice should not be able to reserve book she already has")
	}
	// Bob tries to reserve again (should fail - duplicate reservation)
	if err := db.ReserveBook(bookID, bob); err == nil {
		t.Fatalf("Bob should not be able to make duplicate reservation")
	}

	// Step 4: Alice returns book -> automatically assigned to Bob
	returnedBy, _ := db.ReturnBook(bookID)
	if returnedBy != alice {
		t.Fatalf("wrong returned member ID")
	}
	book, _ = db.GetBook(bookID)
	if book.Available || book.BorrowerID != bob {
		t.Fatalf("book should be automatically assigned to Bob")
	}

	// Verify Bob's reservation was fulfilled and queue updated
	reservations, _ = db.GetReservations(bookID)
	if len(reservations) != 2 || reservations[0].ID != charlie {
		t.Fatalf("queue should now have Charlie first")
	}

	// Step 5: Charlie cancels his reservation
	if err := db.CancelReservation(bookID, charlie); err != nil {
		t.Fatalf("Charlie's cancellation failed: %v", err)
	}
	reservations, _ = db.GetReservations(bookID)
	if len(reservations) != 1 || reservations[0].ID != diana {
		t.Fatalf("queue should now have only Diana")
	}

	// Step 6: Bob returns book -> automatically assigned to Diana (Charlie was skipped)
	_, _ = db.ReturnBook(bookID)
	book, _ = db.GetBook(bookID)
	if book.Available || book.BorrowerID != diana {
		t.Fatalf("book should be automatically assigned to Diana")
	}

	// Step 7: Diana returns book -> no more reservations, book becomes available
	_, _ = db.ReturnBook(bookID)
	book, _ = db.GetBook(bookID)
	if !book.Available {
		t.Fatalf("book should be available after Diana returns it")
	}
	reservations, _ = db.GetReservations(bookID)
	if len(reservations) != 0 {
		t.Fatalf("no reservations should remain")
	}
}

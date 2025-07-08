package library

import (
	"fmt"
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
	memberID, _ := db.AddMember("Alice", "password")

	if err := db.CheckoutBook(bookID, memberID); err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if _, err := db.ReturnBook(bookID); err != nil {
		t.Fatalf("return: %v", err)
	}
}

func TestReservationQueue(t *testing.T) {
	db := tempDB(t)
	bookID, _ := db.AddBook("Book", "Author", "txt")
	alice, _ := db.AddMember("Alice", "password")
	bob, _ := db.AddMember("Bob", "password")

	// Alice checks out the book
	if err := db.CheckoutBook(bookID, alice); err != nil {
		t.Fatalf("checkout: %v", err)
	}

	// Bob reserves the book
	if err := db.ReserveBook(bookID, bob); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	// Alice returns it, should go to Bob
	if returnedBy, err := db.ReturnBook(bookID); err != nil || returnedBy != alice {
		t.Fatalf("return: %v, returnedBy=%d", err, returnedBy)
	}

	// Verify Bob now has it
	book, _ := db.GetBook(bookID)
	if book.Available || book.BorrowerID != bob {
		t.Fatalf("book should be with Bob")
	}
}

// TestPasswordAuthentication tests the password hashing and authentication system.
func TestPasswordAuthentication(t *testing.T) {
	db := tempDB(t)

	// Test adding member with password
	memberID, err := db.AddMember("Alice", "securepassword123")
	if err != nil {
		t.Fatalf("add member with password: %v", err)
	}

	// Test successful authentication
	if err := db.AuthenticateMember(memberID, "securepassword123"); err != nil {
		t.Fatalf("authentication should succeed: %v", err)
	}

	// Test failed authentication with wrong password
	if err := db.AuthenticateMember(memberID, "wrongpassword"); err == nil {
		t.Fatalf("authentication should fail with wrong password")
	}

	// Test authentication with non-existent member
	if err := db.AuthenticateMember(99999, "anypassword"); err == nil {
		t.Fatalf("authentication should fail for non-existent member")
	}
}

// TestPasswordReset tests the password reset functionality.
func TestPasswordReset(t *testing.T) {
	db := tempDB(t)

	// Add member with initial password
	memberID, err := db.AddMember("Bob", "oldpassword")
	if err != nil {
		t.Fatalf("add member: %v", err)
	}

	// Verify old password works
	if err := db.AuthenticateMember(memberID, "oldpassword"); err != nil {
		t.Fatalf("old password should work: %v", err)
	}

	// Reset password
	if err := db.ResetMemberPassword(memberID, "newpassword123"); err != nil {
		t.Fatalf("reset password: %v", err)
	}

	// Verify old password no longer works
	if err := db.AuthenticateMember(memberID, "oldpassword"); err == nil {
		t.Fatalf("old password should no longer work")
	}

	// Verify new password works
	if err := db.AuthenticateMember(memberID, "newpassword123"); err != nil {
		t.Fatalf("new password should work: %v", err)
	}

	// Test reset for non-existent member
	if err := db.ResetMemberPassword(99999, "anypassword"); err == nil {
		t.Fatalf("reset should fail for non-existent member")
	}
}

// TestPasswordHashSecurity tests that passwords are properly hashed.
func TestPasswordHashSecurity(t *testing.T) {
	db := tempDB(t)

	password := "mysecretpassword"
	memberID, err := db.AddMember("Charlie", password)
	if err != nil {
		t.Fatalf("add member: %v", err)
	}

	// Get the member to check the hash
	member, err := db.GetMember(memberID)
	if err != nil {
		t.Fatalf("get member: %v", err)
	}

	// Verify password is not stored in plain text
	if strings.Contains(member.PasswordHash, password) {
		t.Fatalf("password should not be stored in plain text")
	}

	// Verify hash is not empty
	if member.PasswordHash == "" {
		t.Fatalf("password hash should not be empty")
	}

	// Verify hash starts with bcrypt prefix
	if !strings.HasPrefix(member.PasswordHash, "$2") {
		t.Fatalf("password hash should use bcrypt format")
	}
}

// TestAuthenticatedOperations tests that operations requiring authentication work correctly.
func TestAuthenticatedOperations(t *testing.T) {
	db := tempDB(t)

	// Setup
	bookID, _ := db.AddBook("Test Book", "Author", "content")
	memberID, _ := db.AddMember("Alice", "password123")
	otherMemberID, _ := db.AddMember("Bob", "password456")

	// Test authenticated checkout
	if err := db.AuthenticateMember(memberID, "password123"); err != nil {
		t.Fatalf("authentication should succeed: %v", err)
	}
	if err := db.CheckoutBook(bookID, memberID); err != nil {
		t.Fatalf("checkout should succeed: %v", err)
	}

	// Test authenticated reservation by other member
	if err := db.AuthenticateMember(otherMemberID, "password456"); err != nil {
		t.Fatalf("authentication should succeed: %v", err)
	}
	if err := db.ReserveBook(bookID, otherMemberID); err != nil {
		t.Fatalf("reservation should succeed: %v", err)
	}

	// Test authenticated return
	if err := db.AuthenticateMember(memberID, "password123"); err != nil {
		t.Fatalf("authentication should succeed: %v", err)
	}
	if _, err := db.ReturnBook(bookID); err != nil {
		t.Fatalf("return should succeed: %v", err)
	}
}

// TestEmptyPasswordHandling tests handling of empty passwords.
func TestEmptyPasswordHandling(t *testing.T) {
	db := tempDB(t)

	// Test that empty password is rejected during member creation
	_, err := db.AddMember("TestUser", "")
	if err == nil {
		t.Fatalf("should reject empty password during member creation")
	}

	// Test that whitespace-only password is rejected
	_, err = db.AddMember("TestUser", "   ")
	if err == nil {
		t.Fatalf("should reject whitespace-only password")
	}
}

// TestConcurrentAuthentication tests that concurrent authentication attempts are handled safely.
func TestConcurrentAuthentication(t *testing.T) {
	db := tempDB(t)

	memberID, err := db.AddMember("ConcurrentUser", "testpass123")
	if err != nil {
		t.Fatalf("add member: %v", err)
	}

	// Test concurrent authentication attempts
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			done <- db.AuthenticateMember(memberID, "testpass123")
		}()
	}

	// All should succeed
	for i := 0; i < 10; i++ {
		if err := <-done; err != nil {
			t.Fatalf("concurrent authentication failed: %v", err)
		}
	}
}

// TestPasswordComplexity tests various password scenarios.
func TestPasswordComplexity(t *testing.T) {
	db := tempDB(t)

	testCases := []struct {
		name          string
		password      string
		shouldSucceed bool
	}{
		{"simple password", "password", true},
		{"complex password", "MyC0mpl3x!P@ssw0rd", true},
		{"numeric password", "123456789", true},
		{"special chars", "!@#$%^&*()", true},
		{"unicode password", "пароль123", true},
		{"long password", strings.Repeat("a", 100), true},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			memberName := fmt.Sprintf("TestUser%d", i)
			memberID, err := db.AddMember(memberName, tc.password)

			if tc.shouldSucceed {
				if err != nil {
					t.Fatalf("should accept password %q: %v", tc.password, err)
				}
				// Test authentication works
				if err := db.AuthenticateMember(memberID, tc.password); err != nil {
					t.Fatalf("authentication should work for password %q: %v", tc.password, err)
				}
			} else {
				if err == nil {
					t.Fatalf("should reject password %q", tc.password)
				}
			}
		})
	}
}

// TestReservationSystem covers common reservation scenarios.
func TestReservationSystem(t *testing.T) {
	db := tempDB(t)

	book1ID, _ := db.AddBook("Book 1", "Author", "content1")
	book2ID, _ := db.AddBook("Book 2", "Author", "content2")

	member1ID, err := db.AddMember("Alice", "password")
	if err != nil {
		t.Fatalf("add member 1: %v", err)
	}
	member2ID, err := db.AddMember("Bob", "password")
	if err != nil {
		t.Fatalf("add member 2: %v", err)
	}
	member3ID, err := db.AddMember("Charlie", "password")
	if err != nil {
		t.Fatalf("add member 3: %v", err)
	}

	// Test 1: Normal reservation flow
	if err := db.CheckoutBook(book1ID, member1ID); err != nil {
		t.Fatalf("checkout book1: %v", err)
	}

	// Member 2 reserves book1
	if err := db.ReserveBook(book1ID, member2ID); err != nil {
		t.Fatalf("reserve book1: %v", err)
	}

	// Member 3 also reserves book1
	if err := db.ReserveBook(book1ID, member3ID); err != nil {
		t.Fatalf("reserve book1 by member3: %v", err)
	}

	// Member 1 returns book1 - should go to member 2
	returnedBy, err := db.ReturnBook(book1ID)
	if err != nil {
		t.Fatalf("return book1: %v", err)
	}
	if returnedBy != member1ID {
		t.Fatalf("wrong returner: got %d, want %d", returnedBy, member1ID)
	}

	// Check that book1 is now with member2
	book1, _ := db.GetBook(book1ID)
	if book1.Available {
		t.Fatalf("book1 should not be available")
	}
	if book1.BorrowerID != member2ID {
		t.Fatalf("book1 should be with member2, got %d", book1.BorrowerID)
	}

	// Check reservations queue - should only have member3 now
	reservations, _ := db.GetReservations(book1ID)
	if len(reservations) != 1 || reservations[0].ID != member3ID {
		t.Fatalf("wrong reservations queue")
	}

	// Test 2: Cancel reservation
	if err := db.CancelReservation(book1ID, member3ID); err != nil {
		t.Fatalf("cancel reservation: %v", err)
	}

	// Test 3: Return with no queue - should become available
	if _, err := db.ReturnBook(book1ID); err != nil {
		t.Fatalf("return book1 again: %v", err)
	}

	book1, _ = db.GetBook(book1ID)
	if !book1.Available {
		t.Fatalf("book1 should be available")
	}

	// Test 4: Reserve available book - should auto-checkout
	member4ID, _ := db.AddMember("David", "password")
	if err := db.ReserveBook(book2ID, member4ID); err != nil {
		t.Fatalf("reserve available book: %v", err)
	}

	book2, _ := db.GetBook(book2ID)
	if book2.Available {
		t.Fatalf("book2 should be checked out")
	}
	if book2.BorrowerID != member4ID {
		t.Fatalf("book2 should be with member4ID")
	}
}

func TestMemberOperations(t *testing.T) {
	db := tempDB(t)
	bookID, _ := db.AddBook("Book", "Author", "content")
	otherMemberID, _ := db.AddMember("Bob", "password")

	// Bob checks out the book
	if err := db.CheckoutBook(bookID, otherMemberID); err != nil {
		t.Fatalf("checkout: %v", err)
	}

	// Try to get member reservations for someone with no reservations
	reservations, err := db.GetMemberReservations(otherMemberID)
	if err != nil {
		t.Fatalf("get member reservations: %v", err)
	}
	if len(reservations) != 0 {
		t.Fatalf("should have no reservations, got %d", len(reservations))
	}
}

func TestDuplicateReservation(t *testing.T) {
	db := tempDB(t)
	bookID, _ := db.AddBook("Book", "Author", "content")
	mem1, _ := db.AddMember("Alice", "password")
	mem2, _ := db.AddMember("Bob", "password")

	// First member checks out the book
	if err := db.CheckoutBook(bookID, mem1); err != nil {
		t.Fatalf("checkout: %v", err)
	}

	// Second member reserves
	if err := db.ReserveBook(bookID, mem2); err != nil {
		t.Fatalf("first reserve: %v", err)
	}

	// Second member tries to reserve again - should fail
	if err := db.ReserveBook(bookID, mem2); err == nil {
		t.Fatalf("duplicate reservation should fail")
	}
}

func TestReturnNonCheckedOutBook(t *testing.T) {
	db := tempDB(t)
	bookID, _ := db.AddBook("Book", "Author", "content")
	mem1, _ := db.AddMember("Alice", "password")

	// Try to return a book that's not checked out
	if _, err := db.ReturnBook(bookID); err == nil {
		t.Fatalf("should not be able to return non-checked-out book")
	}

	// Check out and return once
	if err := db.CheckoutBook(bookID, mem1); err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if _, err := db.ReturnBook(bookID); err != nil {
		t.Fatalf("return: %v", err)
	}

	// Try to return again
	if _, err := db.ReturnBook(bookID); err == nil {
		t.Fatalf("should not be able to return book twice")
	}
}

func TestCancelNonExistentReservation(t *testing.T) {
	db := tempDB(t)
	bookID, _ := db.AddBook("Book", "Author", "content")
	member1ID, _ := db.AddMember("Alice", "password")
	member2ID, _ := db.AddMember("Bob", "password")

	// Try to cancel reservation that doesn't exist
	if err := db.CancelReservation(bookID, member1ID); err == nil {
		t.Fatalf("should not be able to cancel non-existent reservation")
	}

	// Make a reservation for member1, then try to cancel as member2
	if err := db.CheckoutBook(bookID, member1ID); err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if err := db.ReserveBook(bookID, member2ID); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := db.CancelReservation(bookID, member1ID); err == nil {
		t.Fatalf("should not be able to cancel someone else's reservation")
	}
}

func TestLargeReservationQueue(t *testing.T) {
	db := tempDB(t)
	bookID, _ := db.AddBook("Popular Book", "Famous Author", "great content")

	// Create many members
	alice, _ := db.AddMember("Alice", "password")
	bob, _ := db.AddMember("Bob", "password")
	charlie, _ := db.AddMember("Charlie", "password")
	diana, _ := db.AddMember("Diana", "password")

	// Alice checks out the book
	if err := db.CheckoutBook(bookID, alice); err != nil {
		t.Fatalf("checkout: %v", err)
	}

	// Others reserve in order
	members := []int64{bob, charlie, diana}
	for _, memberID := range members {
		if err := db.ReserveBook(bookID, memberID); err != nil {
			t.Fatalf("reserve by member %d: %v", memberID, err)
		}
	}

	// Check queue order
	reservations, err := db.GetReservations(bookID)
	if err != nil {
		t.Fatalf("get reservations: %v", err)
	}
	if len(reservations) != 3 {
		t.Fatalf("wrong queue length: got %d, want 3", len(reservations))
	}

	// Verify order
	expectedOrder := []int64{bob, charlie, diana}
	for i, expected := range expectedOrder {
		if reservations[i].ID != expected {
			t.Fatalf("wrong order at position %d: got %d, want %d", i, reservations[i].ID, expected)
		}
	}

	// Alice returns - should go to Bob
	if returnedBy, err := db.ReturnBook(bookID); err != nil || returnedBy != alice {
		t.Fatalf("return failed: %v, returnedBy=%d", err, returnedBy)
	}

	// Verify Bob has it now
	book, _ := db.GetBook(bookID)
	if book.Available || book.BorrowerID != bob {
		t.Fatalf("book should be with Bob")
	}

	// Check remaining queue
	reservations, _ = db.GetReservations(bookID)
	if len(reservations) != 2 {
		t.Fatalf("queue should have 2 people left, got %d", len(reservations))
	}
	if reservations[0].ID != charlie || reservations[1].ID != diana {
		t.Fatalf("wrong remaining queue order")
	}
}

func TestGetMemberReservations(t *testing.T) {
	db := tempDB(t)
	b1, _ := db.AddBook("B1", "A1", "c")
	b2, _ := db.AddBook("B2", "A2", "c")
	mem1, _ := db.AddMember("Alice", "password")
	mem2, _ := db.AddMember("Bob", "password")

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
	member1ID, _ := db.AddMember("Alice", "password")
	member2ID, _ := db.AddMember("Bob", "password")

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
	alice, _ := db.AddMember("Alice", "password")
	bob, _ := db.AddMember("Bob", "password")
	charlie, _ := db.AddMember("Charlie", "password")
	diana, _ := db.AddMember("Diana", "password")

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

package library

// Book represents a book in the library.
type Book struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Author     string `json:"author"`
	Content    string `json:"content"`
	Available  bool   `json:"available"`
	BorrowerID int64  `json:"borrower_id,omitempty"`
}

// Member represents a library member with secure password handling.
type Member struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"` // Excluded from JSON serialization for security
}

// LibraryData represents the complete library state for persistence
type LibraryData struct {
	Books           map[string]*Book    `json:"books"`
	Members         map[string]*Member  `json:"members"`
	NextBookID      int                 `json:"next_book_id"`
	NextMemberID    int                 `json:"next_member_id"`
	CheckedOutBooks map[string][]string `json:"checked_out_books"`
}

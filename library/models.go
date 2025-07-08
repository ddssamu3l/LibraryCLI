package library

// Book represents metadata and current availability of a book in the library.
// The full text of the book is stored in the `content` column in the SQLite database.
type Book struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Author     string `json:"author"`
	Content    string `json:"content"`
	Available  bool   `json:"available"`
	BorrowerID int64  `json:"borrower_id"`
}

// Member represents a registered library member.
type Member struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"` // Don't serialize password hash
}

// LibraryData represents the complete library state for persistence
type LibraryData struct {
	Books           map[string]*Book    `json:"books"`
	Members         map[string]*Member  `json:"members"`
	NextBookID      int                 `json:"next_book_id"`
	NextMemberID    int                 `json:"next_member_id"`
	CheckedOutBooks map[string][]string `json:"checked_out_books"`
}

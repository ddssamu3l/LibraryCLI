# Base Database - Library Management System

This directory contains a clean base database for the Library Management System with 16 pre-imported books and no users or reservations.

## Database Contents

### Books (16 total)
- **ID 1**: 1984 by George Orwell
- **ID 2**: Animal Farm by George Orwell
- **ID 3**: The Diary of a Young Girl by Anne Frank
- **ID 4**: The Art of War by Sun Tzu
- **ID 5**: The Fellowship of the Ring by J.R.R. Tolkien
- **ID 6**: Harry Potter and the Chamber of Secrets by J.K. Rowling
- **ID 7**: Harry Potter and the Deathly Hallows by J.K. Rowling
- **ID 8**: Harry Potter and the Half-Blood Prince by J.K. Rowling
- **ID 9**: Harry Potter and the Order of the Phoenix by J.K. Rowling
- **ID 10**: Harry Potter and the Prisoner of Azkaban by J.K. Rowling
- **ID 11**: Harry Potter and the Philosopher's Stone by J.K. Rowling
- **ID 12**: The Return of the King by J.R.R. Tolkien
- **ID 13**: Romeo and Juliet by William Shakespeare
- **ID 14**: The Two Towers by J.R.R. Tolkien
- **ID 15**: The Three Little Pigs by Traditional
- **ID 16**: The Three Musketeers by Alexandre Dumas

### Database State
- ✅ All books available (not checked out)
- ✅ No members registered
- ✅ No reservations
- ✅ FTS5 full-text search enabled
- ✅ Authentication schema ready
- ✅ All books have full content loaded

## How to Restore

### Option 1: Use the restore script
```bash
./restore_base_db.sh
```

### Option 2: Manual restore
```bash
# Remove current database
rm -f library.db library.db-shm library.db-wal

# Copy base database
cp base_db/library.db .
```

## When to Use

This base database is perfect for:
- **Fresh starts**: Clean slate for new testing or demos
- **Reset after experiments**: Restore known good state
- **Multiple environments**: Consistent starting point
- **Training/demos**: Predictable book collection

## Database Schema Version

- **Schema Version**: 3
- **Features**: Authentication, reservations, FTS5 search
- **Compatibility**: Works with current application version

Created: $(date)
Books imported from: texts/ directory 
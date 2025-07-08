# Library Management CLI

## Setup

1. **Download and Extract**
   ```bash
   # Extract the project zip file to your desired location
   cd LibraryCLI
   ```

2. **Install Dependencies**
   ```bash
   go mod tidy
   ```

3. **Initialize Database** (First time setup)
   ```bash
   # Import sample books from the texts/ directory
   go run import_books.go
   ```

## Running the Application

Start the interactive CLI:
```bash
go run -tags sqlite_fts5 .
```

The application will display a welcome message and prompt for commands. Type `help` to see available commands.

## Testing

Run the comprehensive test suite:
```bash
go test -tags sqlite_fts5 ./library
```

The tests cover:
- Authentication and password security
- Book operations and reservations
- Database migrations and schema
- Edge cases and error handling

## Database Management

### Reset to Clean State

To restore the database to a clean state with sample books but no members or reservations:

```bash
./restore_base_db.sh
```

This will:
- Remove current database files
- Restore a clean database with 16 pre-loaded books
- Reset all member accounts and reservations

### Manual Database Reset

If the restore script doesn't work:
```bash
# Remove current database
rm -f library.db library.db-shm library.db-wal

# Copy base database
cp base_db/library.db .
```

### Complete Fresh Start

To start completely from scratch:
```bash
# Remove all database files
rm -f library.db library.db-shm library.db-wal

# Re-import books
go run import_books.go
```

## Sample Books Included

The system comes with 16 classic books pre-loaded:
- Literature: 1984, Animal Farm, Romeo & Juliet, Three Musketeers
- Fantasy: Lord of the Rings trilogy, Harry Potter series
- Historical: Anne Frank's Diary, Art of War
- Children's: Three Little Pigs

## Troubleshooting

### Build Issues
If you encounter build errors:
```bash
# Make sure you're using the sqlite_fts5 build tag
go run -tags sqlite_fts5 .

# Clean and rebuild
go clean
go mod tidy
```

### Database Issues
If the database becomes corrupted:
```bash
# Reset to clean state
./restore_base_db.sh

# Or start fresh
rm -f library.db*
go run import_books.go
```

### Permission Issues
If you can't execute the restore script:
```bash
chmod +x restore_base_db.sh
```

## Architecture

- **Main**: Interactive CLI interface (`main.go`)
- **Manager**: Business logic layer (`library/manager.go`)
- **Database**: SQLite operations with FTS5 (`library/database.go`)
- **Models**: Data structures (`library/models.go`)
- **Tests**: Comprehensive test suite (`library/database_test.go`)

#!/bin/bash

echo "🗄️  Restoring base database..."

# Remove current database files
echo "Cleaning up current database files..."
rm -f library.db library.db-shm library.db-wal

# Copy base database
if [ -f "base_db/library.db" ]; then
    echo "Restoring database from base_db..."
    cp base_db/library.db .
    
    # Copy any additional files if they exist
    if [ -f "base_db/library.db-shm" ]; then
        cp base_db/library.db-shm .
    fi
    if [ -f "base_db/library.db-wal" ]; then
        cp base_db/library.db-wal .
    fi
    
    echo "✅ Base database restored successfully!"
    echo "📚 Your library now has 16 books with no members or reservations."
    echo "🚀 Ready to use: go run -tags sqlite_fts5 ."
else
    echo "❌ Error: base_db/library.db not found!"
    echo "Make sure you're in the correct directory and the base database exists."
    exit 1
fi 
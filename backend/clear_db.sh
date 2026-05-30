#!/bin/bash

echo "=== Clearing Database ==="

# Execute SQL directly inside the running Docker container.
# TRUNCATE CASCADE will clear all tables and reset IDs (SERIAL) to 1.
docker compose exec -T postgres psql -U flibusta_user -d flibusta_db -c "TRUNCATE TABLE users, books, user_books CASCADE;"

echo "✓ Database cleared successfully."

echo ""
echo "=== Clearing downloaded books from storage ==="

# Remove only the contents of the directory, keeping the directory itself
rm -rf ./storage/books/*

# Recreate the directory just in case it was accidentally removed
mkdir -p ./storage/books

echo "✓ Files removed."
echo ""
echo "Done. Ready for testing."
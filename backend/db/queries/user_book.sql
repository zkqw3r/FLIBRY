-- name: AddBookToUser :one
INSERT INTO user_books (
    user_id,
    book_id,
    is_favorite
) VALUES (
    $1, $2, $3
)
ON CONFLICT (user_id, book_id) 
DO UPDATE SET is_favorite = EXCLUDED.is_favorite
RETURNING *;

-- name: UpdateReadingProgress :exec
UPDATE user_books 
SET 
    reading_progress = $1,
    progress_percent = $2,
    last_read_at = NOW()
WHERE user_id = $3 AND book_id = $4;

-- name: GetUserBooks :many
SELECT 
    b.id,
    b.title,
    b.author,
    b.cover_url,
    ub.is_favorite,
    ub.progress_percent,
    ub.last_read_at
FROM user_books ub
JOIN books b ON b.id = ub.book_id
WHERE ub.user_id = $1
ORDER BY ub.last_read_at DESC;

-- name: GetUserBook :one
SELECT * FROM user_books
WHERE user_id = $1 AND book_id = $2
LIMIT 1;

-- name: RemoveBookFromUser :exec
DELETE FROM user_books
WHERE user_id = $1 AND book_id = $2;
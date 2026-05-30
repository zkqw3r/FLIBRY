-- name: GetBookByFlibustaID :one
SELECT * FROM books 
WHERE flibusta_id = $1 
LIMIT 1;

-- name: GetBookByID :one
SELECT * FROM books 
WHERE id = $1 
LIMIT 1;

-- name: CreateBook :one
INSERT INTO books (
    title, 
    author, 
    flibusta_id, 
    cover_url, 
    description
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING *;

-- name: UpdateBookFilePath :exec
UPDATE books 
SET file_path = $1 
WHERE id = $2;

-- name: SearchBooks :many
SELECT * FROM books 
WHERE title ILIKE '%' || $1 || '%' 
   OR author ILIKE '%' || $1 || '%'
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListBooks :many
SELECT * FROM books 
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;
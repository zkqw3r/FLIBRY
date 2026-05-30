-- name: CreateUser :one
INSERT INTO users (
    username, email, password_hash, verification_token
) VALUES (
    $1, $2, $3, $4
)
RETURNING *;

-- name: GetUserByUsername :one
SELECT * FROM users 
WHERE username = $1 
LIMIT 1;

-- name: GetUserByID :one
SELECT * FROM users 
WHERE id = $1 
LIMIT 1;

-- name: VerifyUser :one
UPDATE users 
SET is_verified = true, verification_token = NULL
WHERE verification_token = $1 
RETURNING *;
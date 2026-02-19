-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, username, pass_hash)
VALUES (
    gen_random_uuid(),
    NOW(),
    NOW(),
    $1,
    $2
)
RETURNING id, created_at, updated_at, user_name;
-- name: InsertData :one
INSERT INTO testdata (id, created_at, updated_at, dat, username)
VALUES(
    gen_random_uuid,
    NOW(),
    NOW(),
    $1,
    $2
)
RETURNING id;
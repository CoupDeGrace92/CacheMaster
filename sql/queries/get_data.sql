-- name: GetData :one
SELECT * FROM testdata
WHERE id = $1;
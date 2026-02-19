-- name: GetDataByUser :many
SELECT id FROM testdata
WHERE username = $1;
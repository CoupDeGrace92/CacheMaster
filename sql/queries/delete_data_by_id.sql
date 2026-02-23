-- name: DeleteDataByID :exec
DELETE FROM testdata
WHERE id = $1;
// SPDX-License-Identifier: GPL-3.0-or-later
package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// isUniqueViolation returns true if the error is a PostgreSQL unique constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

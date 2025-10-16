package utils

import "database/sql"

func NilIfNullFloat(f sql.NullFloat64) any {
	if f.Valid {
		return f.Float64
	}
	return nil
}

func NilIfNullString(s sql.NullString) any {
	if s.Valid {
		return s.String
	}
	return nil
}

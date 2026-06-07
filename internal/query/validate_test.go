package query

import "testing"

func TestValidateSQL(t *testing.T) {
	t.Run("allows SELECT", func(t *testing.T) {
		ok := []string{
			"SELECT * FROM data",
			"SELECT name, age FROM data WHERE age > 30",
			"SELECT COUNT(*) FROM data",
			"SELECT AVG(price) FROM data GROUP BY category",
			"  SELECT id FROM data  ",
		}
		for _, q := range ok {
			if err := ValidateSQL(q); err != nil {
				t.Errorf("expected allowed: %q → %v", q, err)
			}
		}
	})

	t.Run("rejects non-SELECT", func(t *testing.T) {
		bad := []string{
			"DROP TABLE data",
			"DELETE FROM data",
			"INSERT INTO data VALUES (1)",
			"UPDATE data SET x=1",
			"CREATE TABLE t AS SELECT 1",
			"ALTER TABLE data ADD COLUMN x INT",
			"TRUNCATE data",
			"COPY data TO '/tmp/out.csv'",
			"ATTACH '/db2' AS db2",
			"PRAGMA database_list",
		}
		for _, q := range bad {
			if err := ValidateSQL(q); err == nil {
				t.Errorf("expected rejected: %q", q)
			}
		}
	})

	t.Run("rejects filesystem table functions", func(t *testing.T) {
		bad := []string{
			"SELECT * FROM read_parquet('/etc/passwd')",
			"SELECT * FROM read_csv('/tmp/secrets.csv')",
			"SELECT * FROM read_json('/tmp/data.json')",
		}
		for _, q := range bad {
			if err := ValidateSQL(q); err == nil {
				t.Errorf("expected rejected: %q", q)
			}
		}
	})

	t.Run("rejects empty", func(t *testing.T) {
		if err := ValidateSQL(""); err == nil {
			t.Error("expected error for empty query")
		}
		if err := ValidateSQL("   "); err == nil {
			t.Error("expected error for whitespace-only query")
		}
	})
}

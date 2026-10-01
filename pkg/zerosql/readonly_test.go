package zerosql

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestRestrictedSupportedSelects(t *testing.T) {
	for _, sql := range []string{
		"SELECT * FROM events",
		"SELECT e.name AS label, e._id FROM events e WHERE e.name = '$cash' ORDER BY label DESC LIMIT 2 OFFSET 1",
		"SELECT NULL AS missing, TRUE AS enabled, -9223372036854775808 AS signed, 1.25 AS decimal FROM events LIMIT 0",
		"SELECT name FROM events WHERE NOT (amount = 1 OR amount IS NULL) AND amount BETWEEN 2 AND 10",
		"SELECT name FROM events WHERE amount NOT IN (1, NULL, 3) OR name NOT LIKE 'a.b'",
		"SELECT name FROM events WHERE amount NOT BETWEEN 1 AND 3 AND name IS NOT NULL",
		"SELECT region, COUNT(*) AS rows, COUNT(amount) AS present, SUM(amount) AS total, AVG(amount) AS average, MIN(amount) AS low, MAX(amount) AS high FROM events GROUP BY region ORDER BY region",
		"SELECT COUNT(*) AS rows, SUM(amount) AS total FROM events WHERE FALSE",
		"SELECT '$cash; DELETE' AS literal FROM events WHERE name = '$cash; DELETE'",
		"SELECT region FROM events GROUP BY region",
	} {
		t.Run(sql, func(t *testing.T) {
			result, err := New(nil).ConvertReadOnlySQLToMongoWithCollection(sql)
			if err != nil {
				t.Fatal(err)
			}
			if result.Collection != "events" {
				t.Fatalf("collection=%q", result.Collection)
			}
			for _, stage := range result.Pipeline {
				encoded, err := json.Marshal(stage)
				if err != nil {
					t.Fatal(err)
				}
				var document bson.D
				if err := bson.UnmarshalExtJSON(encoded, false, &document); err != nil {
					t.Fatal(err)
				}
				if len(document) != 1 {
					t.Fatal("not a single MongoDB stage")
				}
			}
		})
	}
}

func TestRestrictedRejectsUnsupportedSQL(t *testing.T) {
	for _, sql := range []string{
		"", "DELETE FROM events", "UPDATE events SET x = 1", "INSERT INTO events VALUES (1)", "CREATE TABLE events (id INT)",
		"SELECT * FROM events;", "SELECT * FROM events; SELECT * FROM other", "SELECT * FROM events UNION SELECT * FROM other",
		"SELECT * FROM events JOIN other ON events.id=other.id", "SELECT * FROM events, other", "SELECT * FROM (SELECT * FROM events) e",
		"WITH x AS (SELECT * FROM events) SELECT * FROM x", "SELECT * FROM events WHERE x IN (SELECT x FROM other)",
		"SELECT CAST(x AS INT) FROM events", "SELECT LOWER(x) AS value FROM events", "SELECT x + 1 AS value FROM events", "SELECT @x AS value FROM events",
		"SELECT DISTINCT x FROM events", "SELECT x FROM events GROUP BY x HAVING COUNT(*) > 1", "SELECT * FROM events FOR UPDATE",
		"SELECT SQL_CACHE * FROM events", "SELECT * FROM events USE INDEX(x)", "SELECT * FROM db.events", "SELECT * FROM events PARTITION (p)",
		"SELECT x, y AS x FROM events", "SELECT events.* FROM events", "SELECT *, x FROM events", "SELECT 'literal' FROM events", "SELECT x AS `bad.alias` FROM events",
		"SELECT other.x FROM events", "SELECT events.x FROM events e", "SELECT * FROM events ORDER BY x, y", "SELECT * FROM events ORDER BY 1",
		"SELECT * FROM events LIMIT -1", "SELECT * FROM events LIMIT ?", "SELECT * FROM events LIMIT 9223372036854775808",
		"SELECT 9223372036854775808 AS too_large FROM events", "SELECT 1.23456789012345678901234567890123456789 AS too_precise FROM events",
		"SELECT 0xFF AS hex FROM events", "SELECT x FROM events WHERE x <=> NULL", "SELECT x FROM events WHERE x IS TRUE", "SELECT x FROM events WHERE x LIKE 'x' ESCAPE '!'",
		"SELECT COUNT(DISTINCT x) FROM events", "SELECT SUM(*) FROM events", "SELECT COUNT(1) FROM events", "SELECT x, SUM(y) FROM events",
		"SELECT x FROM events GROUP BY y", "SELECT x FROM events GROUP BY x, x", "SELECT SUM(x) AS total FROM events ORDER BY x", "SELECT SUM(x) FROM events GROUP BY x + 1",
		"SELECT name FROM events WHERE name = ?", "SELECT name FROM events WHERE NOT name", "SELECT name FROM events WHERE name IN (other)",
	} {
		t.Run(sql, func(t *testing.T) {
			if _, err := New(nil).ConvertReadOnlySQLToMongoWithCollection(sql); err == nil {
				t.Fatalf("accepted %q", sql)
			}
		})
	}
}

func TestRestrictedBudgets(t *testing.T) {
	for _, sql := range []string{
		"SELECT * FROM events " + strings.Repeat(" ", 64<<10),
		"SELECT * FROM events WHERE " + strings.Repeat("(", 65) + "TRUE" + strings.Repeat(")", 65),
		"SELECT * FROM events WHERE " + strings.Repeat("TRUE OR ", 2100) + "TRUE",
		"SELECT * FROM events WHERE x IN (" + strings.Repeat("1,", 128) + "1)",
		"SELECT * FROM events WHERE (TRUE",
	} {
		if _, err := New(nil).ConvertReadOnlySQLToMongoWithCollection(sql); err == nil {
			t.Fatal("unbounded SQL accepted")
		}
	}
}

func TestRestrictedExactLiteralsAndLiteralDollar(t *testing.T) {
	result, err := New(nil).ConvertReadOnlySQLToMongoWithCollection("SELECT 9007199254740993 AS large, 123456789.01234567890123456789 AS precise, '$cash' AS label FROM events")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result.Pipeline[0])
	if err != nil {
		t.Fatal(err)
	}
	var document bson.Raw
	if err := bson.UnmarshalExtJSON(encoded, false, &document); err != nil {
		t.Fatal(err)
	}
	if got := document.Lookup("$project", "large", "$literal").Int64(); got != 9007199254740993 {
		t.Fatalf("large=%d", got)
	}
	if got := document.Lookup("$project", "precise", "$literal").Decimal128().String(); got != "123456789.01234567890123456789" {
		t.Fatalf("decimal=%s", got)
	}
	if got := document.Lookup("$project", "label", "$literal").StringValue(); got != "$cash" {
		t.Fatalf("label=%s", got)
	}
}

func TestRestrictedCompilerDoesNotReuseMutableLegacyState(t *testing.T) {
	c := New(nil)
	if _, err := c.ConvertSQLToMongo("SELECT name FROM legacy WHERE age > 10"); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for i := 0; i < 20; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			result, err := c.ConvertReadOnlySQLToMongoWithCollection("SELECT * FROM events")
			if err != nil || result.Collection != "events" || len(result.Pipeline) != 0 {
				t.Errorf("restricted compiler reused state: %#v %v", result, err)
			}
		}()
	}
	workers.Wait()
}

func FuzzRestrictedCompiler(f *testing.F) {
	for _, sql := range []string{"SELECT * FROM events", "SELECT COUNT(*) FROM events", "SELECT x FROM events WHERE NOT (x IN (NULL, 1))", "SELECT '$cash' AS literal FROM events", "SELECT * FROM events WHERE x LIKE 'a.b'"} {
		f.Add(sql)
	}
	f.Fuzz(func(t *testing.T, sql string) {
		result, err := New(nil).ConvertReadOnlySQLToMongoWithCollection(sql)
		if err != nil {
			return
		}
		encoded, err := json.Marshal(result.Pipeline)
		if err != nil || len(encoded) > 128<<10 {
			t.Fatal("compiler returned an invalid or unbounded pipeline")
		}
	})
}

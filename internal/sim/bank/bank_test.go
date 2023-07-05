package bank

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ServiceWeaver/weaver/internal/sim"
)

type transfer struct {
	from   string
	to     string
	amount int
}

var genTransfer = sim.Map2(
	sim.OneOf("Alice", "Bob"),
	sim.Int(0, 100),
	func(from string, amount int) transfer {
		if from == "Alice" {
			return transfer{from, "Bob", amount}
		} else {
			return transfer{from, "Alice", amount}
		}
	})

func FuzzBank(f *testing.F) {
	for seed := 0; seed < 10; seed++ {
		f.Add(int64(seed))
	}
	f.Fuzz(func(t *testing.T, seed int64) {
		// Create the database.
		d := t.TempDir()
		dbfile = filepath.Join(d, "db")
		store, err := sql.Open("sqlite3", dbfile)
		if err != nil {
			t.Fatal(err)
		}
		const create = `
			CREATE TABLE IF NOT EXISTS Balances (
				user TEXT PRIMARY KEY,
				balance INTEGER
			);

			INSERT OR IGNORE INTO Balances(user, balance)
			VALUES ('Alice', 100), ('Bob', 100);
		`
		if _, err = store.Exec(create); err != nil {
			t.Fatal(err)
		}

		checkAllBalancesNonNegative := func() error {
			rows, err := store.Query(`SELECT * FROM Balances`)
			if err != nil {
				return err
			}
			for rows.Next() {
				var user string
				var balance int
				if err := rows.Scan(&user, &balance); err != nil {
					return err
				}
				if balance < 0 {
					return fmt.Errorf("user %s has negative balance %d", user, balance)
				}
			}
			return rows.Err()
		}

		// Run the simulation. Bank balances should always be non-negative.
		opts := sim.Options{
			Seed:        seed,
			NumOps:      2,
			NumReplicas: 1,
		}
		s := sim.New(opts)
		sim.RegisterComponent[db](s)
		sim.RegisterComponent[frontend](s)
		sim.RegisterOp(s, "transfer", genTransfer, func(c sim.Caller, ctx context.Context, t transfer) error {
			c.Call("frontend", "Transfer", ctx, t.from, t.to, t.amount)
			return checkAllBalancesNonNegative()
		})
		result := s.Simulate()
		if result.Err != nil {
			t.Log(s.Mermaid())
			t.Fatal(result.Err)
		}
	})
}

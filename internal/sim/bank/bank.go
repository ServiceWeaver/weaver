package bank

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ServiceWeaver/weaver/internal/sim"
	_ "github.com/mattn/go-sqlite3"
)

// In Service Weaver, the db struct would have a config to set the dbfile. We
// fake that with a global variable.
var dbfile string

type db struct {
	caller sim.Caller
	store  *sql.DB
}

func (d *db) SetCaller(caller sim.Caller) {
	d.caller = caller
}

func (d *db) Init() {
	store, err := sql.Open("sqlite3", dbfile)
	if err != nil {
		panic(err)
	}
	d.store = store
}

func (d *db) GetBalance(ctx context.Context, user string) (int, error) {
	const q = `
		SELECT balance
		FROM Balances
		WHERE user = ?;
	`
	var balance int
	err := d.store.QueryRow(q, user).Scan(&balance)
	return balance, err
}

func (d *db) Transfer(ctx context.Context, from, to string, amount int) (struct{}, error) {
	txn, err := d.store.BeginTx(ctx, nil)
	if err != nil {
		return struct{}{}, err
	}
	defer txn.Rollback()

	// Fetch current balances.
	const query = `
		SELECT balance
		FROM Balances
		WHERE user = ?;
	`
	var fromBalance int
	if err := txn.QueryRow(query, from).Scan(&fromBalance); err != nil {
		return struct{}{}, err
	}
	var toBalance int
	if err := txn.QueryRow(query, to).Scan(&toBalance); err != nil {
		return struct{}{}, err
	}

	// Update balances.
	const stmt = `
		UPDATE Balances
		SET balance=?
		WHERE user=?;
	`
	if _, err := txn.Exec(stmt, fromBalance-amount, from); err != nil {
		return struct{}{}, err
	}
	if _, err := txn.Exec(stmt, toBalance+amount, to); err != nil {
		return struct{}{}, err
	}
	return struct{}{}, txn.Commit()
}

type frontend struct {
	caller sim.Caller
}

func (f *frontend) SetCaller(caller sim.Caller) {
	f.caller = caller
}

func (f *frontend) Transfer(ctx context.Context, from, to string, amount int) (struct{}, error) {
	fromBalance, err := f.caller.Call("db", "GetBalance", ctx, from)
	if err != nil {
		return struct{}{}, err
	}
	fb := fromBalance.(int)
	if fb < amount {
		return struct{}{}, fmt.Errorf("insufficient funds %q: got %d, want >= %d", from, fb, amount)
	}
	_, err = f.caller.Call("db", "Transfer", ctx, from, to, amount)
	return struct{}{}, err
}

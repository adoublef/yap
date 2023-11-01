package main

import (
	"context"
	"database/sql"
	"embed"
	"log"
	"os"
	"strings"

	"github.com/maragudk/migrate"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed all:*.up.sql
var sqlFS embed.FS

var (
	args = strings.Join([]string{"_journal=wal", "_timeout=5000", "_synchronous=normal", "_fk=true"}, "&")
	dsn  = os.Getenv("DATABASE_URL")
)

func main() {
	err := run(context.Background())
	if err != nil {
		log.Printf("yap-migrate: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) (err error) {
	db, err := sql.Open("sqlite3", dsn+"?"+args)
	if err != nil {
		return err
	}

	err = migrate.Up(ctx, db, sqlFS)
	if err != nil {
		return err
	}

	return nil
}

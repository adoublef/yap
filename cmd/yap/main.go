package main

import (
	"context"
	"database/sql"
	"embed"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"

	service "github.com/adoublef/yap/internal"
	"github.com/benbjohnson/hashfs"
	"github.com/go-chi/chi/v5"
	_ "github.com/mattn/go-sqlite3"
)

var addr = ":" + os.Getenv("PORT")

//go:embed all:*.css
var assetsFS embed.FS
var hashFS = hashfs.NewFS(assetsFS)

//go:embed all:*.html
var tmplFS embed.FS

var fmap = template.FuncMap{
	"static": func(filename string) string {
		return hashFS.HashName(filename)
	},
	"env": func(key string) string {
		return os.Getenv(key)
	},
}

var (
	args = strings.Join([]string{"_journal=wal", "_timeout=5000", "_synchronous=normal", "_fk=true"}, "&")
	dsn  = os.Getenv("DATABASE_URL")
)

func main() {
	err := run(context.Background())
	if err != nil {
		log.Printf("yap: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) (err error) {
	db, err := sql.Open("sqlite3", dsn+"?"+args)
	if err != nil {
		return err
	}

	t, err := template.New("").Funcs(fmap).ParseFS(tmplFS, "*.html")
	if err != nil {
		return err
	}

	mux := chi.NewMux()
	mux.Mount("/", service.New(db, t))
	mux.Handle("/assets/*", http.StripPrefix("/assets/", hashfs.FileServer(hashFS)))

	return http.ListenAndServe(addr, mux)
}

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

	"github.com/benbjohnson/hashfs"
	"github.com/go-chi/chi/v5"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/xid"
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
	mux.Get("/", handleIndex(t, db))
	mux.Post("/", handlePostYap(db))
	// TODO up/down vote yaps
	// TODO comment on a yap
	// TODO share a yap with a shortened URL
	// TODO about
	mux.Get("/*", hashfs.FileServer(hashFS).ServeHTTP)

	return http.ListenAndServe(addr, mux)
}

type content string

func parseContent(s string) (content, error) {
	return content(s), nil
}

type yap struct {
	ID      xid.ID
	Content content
	Region  string
}

// http server
func handleIndex(t *template.Template, db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		yy, err := listYaps(r.Context(), db)
		if err != nil {
			http.Error(w, "Failed to ping database", http.StatusInternalServerError)
			return
		}

		err = t.ExecuteTemplate(w, "index.html", yy)
		if err != nil {
			http.Error(w, "Failed to render view", http.StatusInternalServerError)
			return
		}
	}
}

func handlePostYap(db *sql.DB) http.HandlerFunc {
	var (
		region = os.Getenv("FLY_REGION")
	)
	return func(w http.ResponseWriter, r *http.Request) {
		content, err := parseContent(r.PostFormValue("content"))
		if err != nil {
			// return with error page
			http.Error(w, "Error with user input", http.StatusBadRequest)
			return
		}

		err = postYap(r.Context(), db, &yap{xid.New(), content, region})
		if err != nil {
			http.Error(w, "Database found an issue", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, r.Referer(), http.StatusSeeOther)
	}
}

// database
func listYaps(ctx context.Context, db *sql.DB) ([]*yap, error) {
	var (
		qry = "SELECT y.id, y.content, y.region FROM yaps AS y"
	)
	rs, err := db.QueryContext(ctx, qry)
	if err != nil {
		return nil, err
	}
	defer rs.Close()

	var vv []*yap
	for rs.Next() {
		var v yap
		err = rs.Scan(&v.ID, &v.Content, &v.Region)
		if err != nil {
			return nil, err
		}
		vv = append(vv, &v)
	}
	return vv, rs.Err()
}

func postYap(ctx context.Context, db *sql.DB, y *yap) (err error) {
	var (
		qry = "INSERT INTO yaps (id, content) VALUES (?, ?)"
	)
	_, err = db.ExecContext(ctx, qry, y.ID, y.Content)
	return
}

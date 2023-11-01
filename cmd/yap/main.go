package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/benbjohnson/hashfs"
	"github.com/go-chi/chi/v5"
	"github.com/gofrs/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/xid"
	xsrf "golang.org/x/net/xsrftoken"
)

var addr = ":" + os.Getenv("PORT")
var hmacSecret = os.Getenv("HMAC_SECRET")

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
	if ok := len(s) <= 240; !ok {
		return content(""), errors.New("content: value too long")
	}
	// NOTE other parsing can be handled here
	return content(s), nil
}

type region string

func parseRegion(s string) (region, error) {
	// lhr, syd & iad
	return region(s), nil
}

type yap struct {
	ID      xid.ID
	Content content
	Region  string
}

// http
func handleIndex(t *template.Template, db *sql.DB) http.HandlerFunc {
	var session = func(w http.ResponseWriter, r *http.Request) string {
		// create a session cookie
		c := http.Cookie{
			Name:     "site-session",
			Value:    uuid.Must(uuid.NewGen().NewV7()).String(),
			Path:     "/",
			Secure:   true,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		}
		http.SetCookie(w, &c)
		return c.Value
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// if no session_id create one
		tok := xsrf.Generate(hmacSecret, session(w, r), "")

		yy, err := listYaps(r.Context(), db)
		if err != nil {
			http.Error(w, "Failed to ping database", http.StatusInternalServerError)
			return
		}

		err = t.ExecuteTemplate(w, "index.html", map[string]any{"Yaps": yy, "Xsrf": tok})
		if err != nil {
			http.Error(w, "Failed to render view", http.StatusInternalServerError)
			return
		}
	}
}

func handlePostYap(db *sql.DB) http.HandlerFunc {
	var region = os.Getenv("FLY_REGION")

	var session = func(w http.ResponseWriter, r *http.Request) string {
		c, err := r.Cookie("site-session")
		if err != nil {
			return ""
		}
		return c.Value
	}

	return func(w http.ResponseWriter, r *http.Request) {
		var (
			content = r.PostFormValue("content")
			tok     = r.PostFormValue("_xsrf")
		)

		// validate token
		if ok := xsrf.Valid(tok, hmacSecret, session(w, r), ""); !ok {
			http.Error(w, "Request has been tampered", http.StatusUnauthorized)
			return
		}

		c, err := parseContent(content)
		if err != nil {
			// return with error page
			http.Error(w, "Error with user input", http.StatusBadRequest)
			return
		}

		err = postYap(r.Context(), db, &yap{xid.New(), c, region})
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
		qry = "INSERT INTO yaps (id, content, region) VALUES (?, ?, ?)"
	)
	_, err = db.ExecContext(ctx, qry, y.ID, y.Content, y.Region)
	return
}
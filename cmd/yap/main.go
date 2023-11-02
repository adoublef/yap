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
	"slices"
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
	mux.Post("/{yap}/vote/{vote}", handleVote(db))
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

// region must be either 'lhr', 'syd' or 'iad' in correspondence to the deployed regions
type region string

func parseRegion(s string) (region, error) {
	// lhr, syd & iad
	if ok := slices.Contains([]string{"lhr", "syd", "iad"}, s); !ok {
		return "", errors.New("region: invalid region")
	}
	return region(s), nil
}

type Yap struct {
	ID      xid.ID
	Content content
	Region  region
	Score   int
}

// http
func handleIndex(t *template.Template, db *sql.DB) http.HandlerFunc {
	var session = func(w http.ResponseWriter, r *http.Request) string {
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
	var session = func(w http.ResponseWriter, r *http.Request) string {
		c, err := r.Cookie("site-session")
		if err != nil {
			return ""
		}
		return c.Value
	}
	var parse = func(r *http.Request) (*Yap, error) {
		c, err := parseContent(r.PostFormValue("content"))
		if err != nil {
			return nil, err
		}

		reg, err := parseRegion(r.PostFormValue("region"))
		if err != nil {
			return nil, err
		}

		return &Yap{xid.New(), c, reg, 0}, nil
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if ok := xsrf.Valid(r.PostFormValue("_xsrf"), hmacSecret, session(w, r), ""); !ok {
			http.Error(w, "Request has been tampered", http.StatusUnauthorized)
			return
		}

		y, err := parse(r)
		if err != nil {
			http.Error(w, "Error with user input", http.StatusBadRequest)
			return
		}

		err = postYap(r.Context(), db, y)
		if err != nil {
			http.Error(w, "Database found an issue", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func handleVote(db *sql.DB) http.HandlerFunc {
	var session = func(w http.ResponseWriter, r *http.Request) string {
		c, err := r.Cookie("site-session")
		if err != nil {
			return ""
		}
		return c.Value
	}
	var parse = func(r *http.Request) (yap xid.ID, upvote bool, err error) {
		yap, err = xid.FromString(chi.URLParam(r, "yap"))
		if err != nil {
			return xid.NilID(), false, err
		}
		vote := chi.URLParam(r, "vote")
		if ok := slices.Contains([]string{"up", "down"}, vote); !ok {
			return xid.NilID(), false, errors.New("parse: invalid vote option")
		}
		upvote = vote == "up"
		return
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if ok := xsrf.Valid(r.PostFormValue("_xsrf"), hmacSecret, session(w, r), ""); !ok {
			http.Error(w, "Request has been tampered", http.StatusUnauthorized)
			return
		}

		yid, upvote, err := parse(r)
		if err != nil {
			http.Error(w, "Error with voting", http.StatusBadRequest)
			return
		}

		err = makeVote(r.Context(), db, yid, upvote)
		if err != nil {
			http.Error(w, "Database found an issue", http.StatusInternalServerError)
			return
		}

		// send htmx back with updated score value?
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// database
func listYaps(ctx context.Context, db *sql.DB) ([]*Yap, error) {
	var (
		qry = `
		SELECT
			y.id,
			y.content,
			y.region,
			SUM(CASE
				WHEN v.score = 0 THEN -1 
				WHEN v.score = 1 THEN 1
				ELSE 0 
			END) AS score
		FROM
			yaps AS y
		LEFT JOIN
			votes AS v ON y.id = v.yap
		GROUP BY
			y.id, y.content
		`
	)
	rs, err := db.QueryContext(ctx, qry)
	if err != nil {
		return nil, err
	}
	defer rs.Close()

	var vv []*Yap
	for rs.Next() {
		var v Yap
		err = rs.Scan(&v.ID, &v.Content, &v.Region, &v.Score)
		if err != nil {
			return nil, err
		}
		vv = append(vv, &v)
	}
	return vv, rs.Err()
}

func postYap(ctx context.Context, db *sql.DB, y *Yap) (err error) {
	var (
		qry = `
		INSERT INTO yaps (id, content, region)
		VALUES (?, ?, ?)
		`
	)
	_, err = db.ExecContext(ctx, qry, y.ID, y.Content, y.Region)
	return
}

func makeVote(ctx context.Context, db *sql.DB, yap xid.ID, upvote bool) (err error) {
	var (
		qry = `
		INSERT INTO votes (yap, score)
		VALUES (?, ?)
		`
	)
	_, err = db.ExecContext(ctx, qry, yap, upvote)
	return
}

func currentScore(ctx context.Context, db *sql.DB, yap xid.ID) (n int, err error) {
	var (
		qry = `
		SELECT 
			SUM(CASE
				WHEN v.score = 0 THEN -1 
				WHEN v.score = 1 THEN 1
				ELSE 0 
			END) AS score
		FROM
			votes AS v
		WHERE
			v.yap = ?
		`
	)
	err = db.QueryRowContext(ctx, qry).Scan(&n)
	return
}

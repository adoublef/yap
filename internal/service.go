package internal

import (
	"database/sql"
	"errors"
	"html/template"
	"net/http"
	"os"
	"slices"

	"github.com/adoublef/yap"
	"github.com/go-chi/chi/v5"
	"github.com/gofrs/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/xid"
	xsrf "golang.org/x/net/xsrftoken"
)

var hmacSecret = os.Getenv("HMAC_SECRET")

var _ http.Handler = (*Service)(nil)

type Service struct {
	m  *chi.Mux
	db *sql.DB
	t  *template.Template
}

// ServeHTTP implements http.Handler.
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.m.ServeHTTP(w, r)
}

func New(
	db *sql.DB,
	t *template.Template,
) *Service {
	s := Service{
		m:  chi.NewMux(),
		db: db,
		t:  t,
	}
	s.routes()
	return &s
}

func (s *Service) routes() {
	s.m.Get("/", s.handleIndex())
	s.m.Post("/", s.handlePostYap())
	s.m.Post("/yap/{yap}", s.handleViewYap())
	s.m.Post("/yap/{yap}/vote/{vote}", s.handleVote())
	// TODO comment on a yap
	// TODO share a yap with a shortened URL
	// TODO about
}

func (s *Service) handleIndex() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := xsrf.Generate(hmacSecret, s.setSession(w, r), "")

		yy, err := listYaps(r.Context(), s.db)
		if err != nil {
			http.Error(w, "Failed to ping database", http.StatusInternalServerError)
			return
		}

		s.respond(w, r, "index.html", map[string]any{"Yaps": yy, "Xsrf": tok})
	}
}

func (s *Service) handlePostYap() http.HandlerFunc {
	var parse = func(r *http.Request) (*yap.Yap, error) {
		c, err := yap.ParseContent(r.PostFormValue("content"))
		if err != nil {
			return nil, err
		}
		reg, err := yap.ParseRegion(r.PostFormValue("region"))
		if err != nil {
			return nil, err
		}
		y := yap.Yap{ID: xid.New(), Content: c, Region: reg, Score: 0}
		return &y, nil
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if ok := xsrf.Valid(r.PostFormValue("_xsrf"), hmacSecret, s.getSession(w, r), ""); !ok {
			http.Error(w, "Request has been tampered", http.StatusUnauthorized)
			return
		}

		y, err := parse(r)
		if err != nil {
			http.Error(w, "Error with user input", http.StatusBadRequest)
			return
		}

		err = postYap(r.Context(), s.db, y)
		if err != nil {
			http.Error(w, "Database found an issue", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func (s *Service) handleViewYap() http.HandlerFunc {
	var parse = func(r *http.Request) (yap xid.ID, err error) {
		yap, err = xid.FromString(chi.URLParam(r, "yap"))
		if err != nil {
			return xid.NilID(), err
		}
		return
	}
	return func(w http.ResponseWriter, r *http.Request) {
		yid, err := parse(r)
		if err != nil {
			// pass error to template
			// NOTE may not even need to do error checking
			s.respond(w, r, "error.html", nil)
			return
		}
		s.respond(w, r, "yap.html", yid)
	}
}

func (s *Service) handleVote() http.HandlerFunc {
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
		if ok := xsrf.Valid(r.PostFormValue("_xsrf"), hmacSecret, s.getSession(w, r), ""); !ok {
			http.Error(w, "Request has been tampered", http.StatusUnauthorized)
			return
		}

		yid, upvote, err := parse(r)
		if err != nil {
			http.Error(w, "Error with voting", http.StatusBadRequest)
			return
		}

		err = makeVote(r.Context(), s.db, yid, upvote)
		if err != nil {
			http.Error(w, "Database found an issue", http.StatusInternalServerError)
			return
		}
		// send htmx back with updated score value?
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func (s *Service) respond(w http.ResponseWriter, r *http.Request, name string, data any) {
	err := s.t.ExecuteTemplate(w, name, data)
	if err != nil {
		http.Error(w, "Failed to render view", http.StatusInternalServerError)
		return
	}
}

func (s *Service) setSession(w http.ResponseWriter, r *http.Request) (session string) {
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

func (s *Service) getSession(w http.ResponseWriter, r *http.Request) (session string) {
	c, err := r.Cookie("site-session")
	if err != nil {
		return ""
	}
	return c.Value
}

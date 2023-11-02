package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"log"
	"maps"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	service "github.com/adoublef/yap/internal"
	"github.com/adoublef/yap/static"
	"github.com/go-chi/chi/v5"
	_ "github.com/mattn/go-sqlite3"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

var addr = ":" + os.Getenv("PORT")

//go:embed all:*.html
var tmplFS embed.FS

var fmap = template.FuncMap{
	"env": func(key string) string {
		return os.Getenv(key)
	},
}

var (
	args = strings.Join([]string{"_journal=wal", "_timeout=5000", "_synchronous=normal", "_fk=true"}, "&")
	dsn  = os.Getenv("DATABASE_URL")
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	q := make(chan os.Signal, 1)
	signal.Notify(q, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-q
		cancel()
	}()

	if err := run(ctx); err != nil {
		log.Printf("yap: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) (err error) {
	// start nats server
	ns, err := server.NewServer(&server.Options{})
	if err != nil {
		return err
	}
	// non-blocking
	ns.Start()

	// connect to nats server
	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		return err
	}
	defer nc.Close()

	db, err := sql.Open("sqlite3", dsn+"?"+args)
	if err != nil {
		return err
	}

	maps.Copy(fmap, static.FuncMap)
	t, err := template.New("").Funcs(fmap).ParseFS(tmplFS, "*.html")
	if err != nil {
		return err
	}

	mux := chi.NewMux()
	mux.Handle("/", http.RedirectHandler("/feed", http.StatusFound))
	mux.Mount("/feed", service.New(t, db, nc))
	// sse using NATS.io to send notification to all users
	mux.Handle("/assets/*", http.StripPrefix("/assets/", static.Handler))

	s := &http.Server{
		Addr:    addr,
		Handler: mux,
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
	}

	sErr := make(chan error)
	go func() {
		sErr <- s.ListenAndServe()
	}()

	select {
	case err := <-sErr:
		return fmt.Errorf("main error: starting server: %w", err)
	case <-ctx.Done():
		// TODO
		return s.Shutdown(context.Background())
	}
}

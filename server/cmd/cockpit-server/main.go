// Command cockpit-server is the Docker Cockpit controller.
//
//	cockpit-server                 run the controller (same as "serve")
//	cockpit-server passwd          set the dashboard admin password
//	cockpit-server host add NAME   register a host and print its agent token
//	cockpit-server host list       list hosts
//	cockpit-server host token ID   issue a new token for a host
//	cockpit-server host rm ID      remove a host
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"dockpit/server/internal/api"
	"dockpit/server/internal/auth"
	"dockpit/server/internal/hosts"
	"dockpit/server/internal/storage"
	"dockpit/server/internal/webui"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `usage:
  cockpit-server [serve]          run the controller
  cockpit-server passwd           set the dashboard admin password (reads stdin)
  cockpit-server passwd -check    exit 1 if no admin password is set
  cockpit-server host add NAME    register a host and print its agent token
  cockpit-server host list        list hosts
  cockpit-server host token ID    issue a new agent token for a host
  cockpit-server host rm ID       remove a host and revoke its token

environment:
  COCKPIT_DB       database path (default cockpit.db)
  COCKPIT_LISTEN   listen address (default 127.0.0.1:8080)
  COCKPIT_WEB_DIR  serve the web UI from this directory instead of the embedded one
`

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(logger, os.Args[1:]); err != nil {
		logger.Error("cockpit-server", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, args []string) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		fmt.Print(usage)
		return nil
	}

	store, err := storage.Open(envOr("COCKPIT_DB", "cockpit.db"))
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer store.Close()

	switch cmd {
	case "serve":
		return serve(logger, store)
	case "passwd":
		if len(args) == 1 && args[0] == "-check" {
			return passwdCheck(store)
		}
		return passwd(store)
	case "host":
		return hostCmd(store, args)
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func serve(logger *slog.Logger, store *storage.Store) error {
	listen := envOr("COCKPIT_LISTEN", "127.0.0.1:8080")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if hash, err := store.AdminPasswordHash(ctx); err != nil {
		return err
	} else if hash == "" {
		logger.Warn("no admin password set: the dashboard cannot be used until you run 'cockpit-server passwd'")
	}

	web, source := webHandler()
	if web == nil {
		logger.Warn("no web UI: build it with 'make web' or use the Vite dev server")
	} else {
		logger.Info("serving web UI", "from", source)
	}

	srv := api.New(store, hosts.NewRegistry(), logger, web)
	httpServer := &http.Server{
		Addr:              listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// Hijacked WebSocket connections are not closed by Shutdown, so tie
		// them to ctx instead.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("controller listening", "addr", listen, "version", version)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// webHandler picks the UI to serve: COCKPIT_WEB_DIR if set, else the UI
// embedded in the binary, else web/dist when running from the repository.
func webHandler() (http.Handler, string) {
	if dir := os.Getenv("COCKPIT_WEB_DIR"); dir != "" {
		return http.FileServerFS(os.DirFS(dir)), dir
	}
	if h, ok := webui.Handler(); ok {
		return h, "embedded"
	}
	if fi, err := os.Stat("web/dist"); err == nil && fi.IsDir() {
		return http.FileServerFS(os.DirFS("web/dist")), "web/dist"
	}
	return nil, ""
}

func passwd(store *storage.Store) error {
	password, err := readPassword()
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	ctx := context.Background()
	if err := store.SetAdminPasswordHash(ctx, hash); err != nil {
		return err
	}
	// A new password ends all existing dashboard sessions.
	if err := store.DeleteAllSessions(ctx); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "admin password updated")
	return nil
}

// passwdCheck exits non-zero when no admin password is set.
func passwdCheck(store *storage.Store) error {
	hash, err := store.AdminPasswordHash(context.Background())
	if err != nil {
		return err
	}
	if hash == "" {
		os.Exit(1)
	}
	return nil
}

// readPassword prompts twice on a terminal, or reads one line from a pipe.
func readPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", errors.New("no password on stdin")
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	fmt.Fprintf(os.Stderr, "New admin password (min %d chars): ", auth.MinPasswordLength)
	p1, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	fmt.Fprint(os.Stderr, "Repeat password: ")
	p2, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	if string(p1) != string(p2) {
		return "", errors.New("passwords do not match")
	}
	return string(p1), nil
}

func hostCmd(store *storage.Store, args []string) error {
	if len(args) == 0 {
		return errors.New("host: missing subcommand (add, list, token, rm)")
	}
	// -q may appear anywhere, e.g. "host add NAME -q".
	sub, quietFlag, rest := args[0], false, []string{}
	for _, a := range args[1:] {
		if a == "-q" {
			quietFlag = true
		} else {
			rest = append(rest, a)
		}
	}
	quiet := &quietFlag
	ctx := context.Background()

	printToken := func(id, token string) {
		if *quiet {
			fmt.Println(token)
			return
		}
		fmt.Printf("host:  %s\ntoken: %s\n\n", id, token)
		fmt.Println("The token is shown only once. Start the agent on that host with:")
		fmt.Printf("  COCKPIT_CONTROLLER_URL=https://<controller> COCKPIT_TOKEN=%s ./cockpit-agent\n", token)
	}

	switch sub {
	case "add":
		if len(rest) != 1 {
			return errors.New("usage: host add NAME")
		}
		h, token, err := hosts.Register(ctx, store, rest[0])
		if err != nil {
			return err
		}
		printToken(h.ID, token)
	case "list":
		list, err := store.ListHosts(ctx)
		if err != nil {
			return err
		}
		for _, h := range list {
			seen := "never"
			if h.LastSeenAt != nil {
				seen = h.LastSeenAt.Format(time.RFC3339)
			}
			fmt.Printf("%-20s %-24s %s/%s  last seen %s\n", h.ID, h.Name, h.Info.OS, h.Info.Arch, seen)
		}
	case "token":
		if len(rest) != 1 {
			return errors.New("usage: host token ID")
		}
		token, err := hosts.RotateToken(ctx, store, rest[0])
		if errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("host %q not found", rest[0])
		}
		if err != nil {
			return err
		}
		printToken(rest[0], token)
	case "rm":
		if len(rest) != 1 {
			return errors.New("usage: host rm ID")
		}
		if err := store.DeleteHost(ctx, rest[0]); err != nil {
			return fmt.Errorf("remove %q: %w", rest[0], err)
		}
		fmt.Fprintf(os.Stderr, "removed %s\n", rest[0])
	default:
		return fmt.Errorf("host: unknown subcommand %q", sub)
	}
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

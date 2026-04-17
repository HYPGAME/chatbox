package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"chatbox/internal/keys"
	"chatbox/internal/session"
	"chatbox/internal/tui"
	"chatbox/internal/update"
	"chatbox/internal/version"
)

var (
	runHostUI                   = tui.RunHost
	runHostHeadless             = func(context.Context, *session.Host, string, []byte) error { return errors.New("headless host is not implemented") }
	runJoinUI                   = tui.RunJoin
	runSelfUpdateCommand        = runSelfUpdate
	launchBackgroundUpdateCheck = func(ctx context.Context) {
		update.StartBackgroundCheck(ctx, update.Client{
			Repository:     "HYPGAME/chatbox",
			CurrentVersion: version.Version,
		}, version.Version, stderr)
	}
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageError()
	}
	if shouldLaunchBackgroundUpdateCheck(args) {
		launchBackgroundUpdateCheck(ctx)
	}

	switch args[0] {
	case "keygen":
		return runKeygen(args[1:])
	case "host":
		return runHost(ctx, args[1:])
	case "join":
		return runJoin(ctx, args[1:])
	case "version":
		return runVersion()
	case "self-update":
		return runSelfUpdateCommand(ctx)
	default:
		return usageError()
	}
}

func runKeygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	out := fs.String("out", "", "Path to the generated PSK file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("keygen requires --out")
	}
	return keys.GeneratePSKFile(*out)
}

func runVersion() error {
	_, err := fmt.Fprintln(stdout, version.Version)
	return err
}

func runSelfUpdate(ctx context.Context) error {
	result, err := update.Client{
		Repository:     "HYPGAME/chatbox",
		CurrentVersion: version.Version,
	}.SelfUpdate(ctx)
	if err != nil {
		return err
	}

	switch {
	case result.Updated && result.FallbackPath != "":
		_, err = fmt.Fprintf(stdout, "downloaded %s to %s; replace the current binary manually\n", result.LatestVersion, result.FallbackPath)
	case result.Updated:
		_, err = fmt.Fprintf(stdout, "updated chatbox to %s\n", result.LatestVersion)
	default:
		current := result.CurrentVersion
		if current == "" {
			current = version.Version
		}
		_, err = fmt.Fprintf(stdout, "chatbox is already up to date (%s)\n", current)
	}
	return err
}

func runHost(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("host", flag.ContinueOnError)
	listenAddr := fs.String("listen", "0.0.0.0:7331", "TCP address to listen on")
	pskFile := fs.String("psk-file", "", "Path to the PSK file")
	name := fs.String("name", defaultName(), "Local display name")
	headless := fs.Bool("headless", false, "Run as a non-interactive relay service")
	ui := fs.String("ui", "", "UI mode: scrollback or tui")
	alert := fs.String("alert", "", "Alert mode: bell or off")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pskFile == "" {
		return errors.New("host requires --psk-file")
	}
	if *headless && strings.TrimSpace(*ui) != "" {
		return errors.New("host --headless cannot be combined with --ui")
	}
	uiMode, err := resolveUI(*ui)
	if err != nil {
		return err
	}
	alertMode, err := resolveAlert(*alert)
	if err != nil {
		return err
	}

	psk, err := keys.LoadPSKFromFile(*pskFile)
	if err != nil {
		return err
	}

	host, err := session.Listen(*listenAddr, session.Config{
		Name: *name,
		PSK:  psk,
	})
	if err != nil {
		return err
	}
	defer host.Close()

	if *headless {
		return runHostHeadless(ctx, host, *name, psk)
	}
	return runHostUI(host, *name, psk, uiMode, alertMode)
}

func runJoin(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("join", flag.ContinueOnError)
	peer := fs.String("peer", "", "Remote IP:port to connect to")
	pskFile := fs.String("psk-file", "", "Path to the PSK file")
	name := fs.String("name", defaultName(), "Local display name")
	ui := fs.String("ui", "", "UI mode: scrollback or tui")
	alert := fs.String("alert", "", "Alert mode: bell or off")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *peer == "" {
		return errors.New("join requires --peer")
	}
	if *pskFile == "" {
		return errors.New("join requires --psk-file")
	}
	uiMode, err := resolveUI(*ui)
	if err != nil {
		return err
	}
	alertMode, err := resolveAlert(*alert)
	if err != nil {
		return err
	}

	psk, err := keys.LoadPSKFromFile(*pskFile)
	if err != nil {
		return err
	}

	conn, err := session.Dial(ctx, *peer, session.Config{
		Name: *name,
		PSK:  psk,
	})
	if err != nil {
		return err
	}
	defer conn.Close()

	return runJoinUI(conn, *name, *peer, session.Config{
		Name: *name,
		PSK:  psk,
	}, uiMode, alertMode)
}

func defaultName() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "chatbox"
	}
	return name
}

func usageError() error {
	return errors.New("usage: chatbox <keygen|host|join|version|self-update> [flags]")
}

func shouldLaunchBackgroundUpdateCheck(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if args[0] == "self-update" {
		return false
	}
	if args[0] != "host" {
		return true
	}
	return !hostHeadlessRequested(args[1:])
}

func hostHeadlessRequested(args []string) bool {
	for _, arg := range args {
		switch {
		case arg == "--headless":
			return true
		case strings.HasPrefix(arg, "--headless="):
			value := strings.TrimSpace(strings.TrimPrefix(arg, "--headless="))
			if value == "" {
				return true
			}
			enabled, err := strconv.ParseBool(value)
			if err != nil {
				return false
			}
			return enabled
		}
	}
	return false
}

func resolveUI(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "scrollback":
		return "scrollback", nil
	case "tui":
		return "tui", nil
	default:
		return "", fmt.Errorf("unsupported ui %q: use scrollback or tui", raw)
	}
}

func resolveAlert(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "bell":
		return "bell", nil
	case "off":
		return "off", nil
	default:
		return "", fmt.Errorf("unsupported alert %q: use bell or off", raw)
	}
}

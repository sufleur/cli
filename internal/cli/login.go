package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sufleur/cli/internal/auth"
	"github.com/sufleur/cli/internal/credentials"
)

const defaultAPIBase = "https://api.sufleur.com"

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate as a user via the device-code flow",
	Long:  "Opens a browser to confirm a one-time code, then stores a user API key in $XDG_CONFIG_HOME/sufleur/credentials.yaml for use by the agent command tree.",
	RunE: func(cmd *cobra.Command, args []string) error {
		verbose, _ := cmd.Flags().GetBool("verbose")
		label, _ := cmd.Flags().GetString("label")

		exists, err := credentials.Exists()
		if err != nil {
			return err
		}
		if exists {
			path, _ := credentials.Path()
			return fmt.Errorf("already logged in (%s exists). Run `sufleur logout` first if you want to switch accounts", path)
		}

		if label == "" {
			label = defaultDeviceLabel()
		}

		apiBase := apiBaseURL()
		hc := newHTTPClient(verbose)

		ctx := cmd.Context()
		dc, err := auth.RequestDeviceCode(ctx, hc, apiBase, label)
		if err != nil {
			return err
		}

		printActivationPrompt(cmd.OutOrStdout(), dc)
		openBrowser(activationLink(dc))

		tok, err := pollUntilDone(ctx, cmd.OutOrStdout(), hc, apiBase, dc)
		if err != nil {
			return err
		}

		if err := credentials.Save(credentials.Credentials{
			APIBase: apiBase,
			APIKey:  tok.APIKey,
			UserID:  tok.UserID,
			KeyID:   tok.KeyID,
		}); err != nil {
			return err
		}

		path, _ := credentials.Path()
		fmt.Fprintf(cmd.OutOrStdout(), "\nLogged in. Credentials saved to %s\n", path)
		return nil
	},
}

func init() {
	loginCmd.Flags().String("label", "", "Human-readable label for the issued key (defaults to <hostname> (<goos>))")
}

func apiBaseURL() string {
	if v := os.Getenv("SUFLEUR_API_BASE"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return defaultAPIBase
}

func defaultDeviceLabel() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown-host"
	}
	return fmt.Sprintf("%s (%s)", host, runtime.GOOS)
}

func activationLink(dc *auth.DeviceCode) string {
	if strings.Contains(dc.VerificationURI, "?") {
		return dc.VerificationURI + "&code=" + dc.UserCode
	}
	return dc.VerificationURI + "?code=" + dc.UserCode
}

func printActivationPrompt(w io.Writer, dc *auth.DeviceCode) {
	link := activationLink(dc)
	fmt.Fprintf(w, "\nVisit this URL in your browser to confirm sign-in:\n\n    %s\n\n", link)
	fmt.Fprintf(w, "Or enter the following code manually at %s:\n\n    %s\n\n", dc.VerificationURI, dc.UserCode)
	fmt.Fprintln(w, "Waiting for approval...")
}

// pollUntilDone runs the RFC 8628 polling loop until success, a terminal error,
// or the server-provided expiry window elapses.
func pollUntilDone(ctx context.Context, w io.Writer, hc *http.Client, apiBase string, dc *auth.DeviceCode) (*auth.Token, error) {
	interval := time.Duration(dc.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("login code expired before approval — run `sufleur login` again")
		}

		tok, err := auth.PollToken(ctx, hc, apiBase, dc.DeviceCode)
		if err == nil {
			return tok, nil
		}
		switch {
		case errors.Is(err, auth.ErrAuthorizationPending):
			continue
		case errors.Is(err, auth.ErrSlowDown):
			interval += 5 * time.Second
			continue
		case errors.Is(err, auth.ErrAccessDenied):
			return nil, fmt.Errorf("login denied")
		case errors.Is(err, auth.ErrExpiredToken):
			return nil, fmt.Errorf("login code expired before approval — run `sufleur login` again")
		default:
			return nil, err
		}
	}
}

// newHTTPClient returns an *http.Client that mirrors the verbose-logging
// behaviour of internal/fetcher when --verbose is set.
func newHTTPClient(verbose bool) *http.Client {
	hc := &http.Client{}
	if verbose {
		hc.Transport = &verboseTransport{wrapped: http.DefaultTransport}
	}
	return hc
}

// verboseTransport logs request/response bodies to stderr. Mirrors
// internal/fetcher's debugTransport so --verbose works for the new commands.
type verboseTransport struct {
	wrapped http.RoundTripper
}

func (v *verboseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(body))
		fmt.Fprintf(os.Stderr, "[verbose] → %s %s\n[verbose] Request body: %s\n", req.Method, req.URL, body)
	}
	resp, err := v.wrapped.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body = io.NopCloser(bytes.NewReader(body))
	fmt.Fprintf(os.Stderr, "[verbose] ← %d %s\n", resp.StatusCode, body)
	return resp, nil
}

// openBrowser is best-effort: print the URL on failure but never return an error.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

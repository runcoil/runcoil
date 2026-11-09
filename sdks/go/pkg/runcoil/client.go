package runcoil

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
)

// The environment variables NewClient reads when no option sets the value.
const (
	envURL    = "RUNCOIL_URL"
	envAPIKey = "RUNCOIL_API_KEY" //nolint:gosec // The name of the variable, not a key.
)

// Client is the connection to one server. It is safe for concurrent use, and it can serve
// any number of Workers.
type Client struct {
	logger    *slog.Logger
	apiKey    string
	transport connect.Transport
}

// ClientOption configures a Client in NewClient.
type ClientOption func(*clientCfg)

// clientCfg holds what the environment and the options set, until NewClient builds the
// Client.
type clientCfg struct {
	url    string
	apiKey string
	logger *slog.Logger
}

// NewClient validates the options and returns a Client. The URL and the API key come
// from RUNCOIL_URL and RUNCOIL_API_KEY unless URL and APIKey set them. There is no
// default URL, so a key is never sent to a host the caller did not name. NewClient does
// not contact the server; the first call happens in Worker.Start.
func NewClient(opts ...ClientOption) (*Client, error) {
	cfg := clientCfg{
		url:    os.Getenv(envURL),
		apiKey: os.Getenv(envAPIKey),
		logger: slog.New(slog.DiscardHandler),
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("new client: %w", err)
	}

	// No read limit: a message can be larger than the default 4 MiB, and the SDK holds no
	// copy of the server's limits.
	transport := connecthttp.NewTransport(httpClient(cfg.url), cfg.url, connecthttp.WithReadMaxBytes(0))

	return &Client{logger: cfg.logger, apiKey: cfg.apiKey, transport: transport}, nil
}

// validate checks the URL and the API key. Its errors do not name the function;
// NewClient adds that prefix.
func (cfg *clientCfg) validate() error {
	if cfg.url == "" {
		return fmt.Errorf("url is required: set it with URL or %s", envURL)
	}

	u, err := url.Parse(cfg.url)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url scheme %q must be http or https", u.Scheme)
	}

	if u.Host == "" {
		return errors.New("url has no host")
	}

	if cfg.apiKey == "" {
		return fmt.Errorf("api key is required: set it with APIKey or %s", envAPIKey)
	}

	return nil
}

// httpClient speaks HTTP/2 only, as the worker protocol does: without TLS for an http URL,
// and over TLS for an https one. It has no overall timeout, because a Listen stream is
// open for as long as the worker runs.
func httpClient(rawURL string) *http.Client {
	protocols := new(http.Protocols)
	if u, err := url.Parse(rawURL); err == nil && u.Scheme == "http" {
		protocols.SetUnencryptedHTTP2(true)
	} else {
		protocols.SetHTTP2(true)
	}

	return &http.Client{Transport: &http.Transport{Protocols: protocols}}
}

// URL sets the address of the server's worker protocol, such as http://localhost:7070.
// It must use the http or https scheme. It replaces RUNCOIL_URL.
func URL(u string) ClientOption {
	return func(cfg *clientCfg) { cfg.url = u }
}

// APIKey sets the project's API key, which the worker sends to register. It replaces
// RUNCOIL_API_KEY.
func APIKey(key string) ClientOption {
	return func(cfg *clientCfg) { cfg.apiKey = key }
}

// Logger sets the logger for the SDK's own log lines, such as a lost connection or a
// result that could not be reported. The default discards them, and so does nil.
func Logger(l *slog.Logger) ClientOption {
	return func(cfg *clientCfg) {
		if l == nil {
			l = slog.New(slog.DiscardHandler)
		}

		cfg.logger = l
	}
}

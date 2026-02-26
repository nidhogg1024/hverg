package proxy

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// ReverseProxy wrap httputil.ReverseProxy and adds logic for handling
// the backend target dynamically.
type ReverseProxy struct {
	target *url.URL
	proxy  *httputil.ReverseProxy
}

// NewReverseProxy creates a new ReverseProxy based on the target URL.
// The backend can be something like "http://user-service:8080".
func NewReverseProxy(backendURL string) (*ReverseProxy, error) {
	parsedURL, err := url.Parse(backendURL)
	if err != nil {
		return nil, err
	}

	rp := httputil.NewSingleHostReverseProxy(parsedURL)

	// Custom error handler to avoid proxy returning a generic 502 with no logging
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("Proxy error", "err", err, "target", backendURL)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	return &ReverseProxy{
		target: parsedURL,
		proxy:  rp,
	}, nil
}

// ServeHTTP delegates to the standard ReverseProxy.
func (p *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.proxy.ServeHTTP(w, r)
}

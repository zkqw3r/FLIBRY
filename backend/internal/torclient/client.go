package torclient

import (
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/proxy"
)

// NewTorClient creates an HTTP client configured to route traffic through a Tor SOCKS5 proxy.
func NewTorClient(proxyURL string) (*http.Client, error) {
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}

	dialer, err := proxy.SOCKS5("tcp", parsedURL.Host, nil, proxy.Direct)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		Dial: dialer.Dial,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   180 * time.Second,
	}

	return client, nil
}

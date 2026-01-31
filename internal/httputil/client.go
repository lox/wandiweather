package httputil

import (
	"net/http"
	"time"
)

const DefaultTimeout = 30 * time.Second

// NewClient returns an HTTP client with standard timeout configuration.
func NewClient() *http.Client {
	return &http.Client{
		Timeout: DefaultTimeout,
	}
}

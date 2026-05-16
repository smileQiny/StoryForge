package llm

import (
	"crypto/tls"
	"net/http"
	"time"
)

func newHTTPClient(timeout time.Duration, insecureSkipVerify bool) *http.Client {
	client := &http.Client{Timeout: timeout}
	if !insecureSkipVerify {
		return client
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	transport.TLSClientConfig.InsecureSkipVerify = true
	client.Transport = transport
	return client
}

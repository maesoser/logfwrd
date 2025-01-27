package main

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"time"
)

// Sends syslog entries to a s3 bucket
// GZ Compressed files every 300 seconds or every 10K lines
type httpBuffer struct {
	Verbose     bool
	httpSession http.Client
	Tag         string
	Auth        string
	Endpoint    string
}

// Create a new instance oaf the service's client with a Session.
func (buffer *httpBuffer) Init(endpoint, auth string) {
	buffer.Endpoint = endpoint
	buffer.Auth = auth
	buffer.Verbose = false
	buffer.Tag = ""
	buffer.httpSession = http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			IdleConnTimeout:     15 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}
}

func (buffer *httpBuffer) Add(text string) error {
	req, err := http.NewRequest(http.MethodPost, buffer.Endpoint, bytes.NewBuffer([]byte(text)))
	if err != nil {
		log.Println(err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	clientName := "Logfwrd"
	clientVersion := "0.2"
	goVersion := runtime.Version()
	req.Header.Set("User-Agent", fmt.Sprintf("%s/%s (%s)", clientName, clientVersion, goVersion))
	if buffer.Auth != "" {
		req.Header.Set("Authorization", buffer.Auth)
	}
	if buffer.Tag != "" {
		req.Header.Set("X-Log-Tag", buffer.Tag)
	}
	resp, err := buffer.httpSession.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode > 299 {
		return fmt.Errorf("received non-200 response: %d", resp.StatusCode)
	}
	return nil
}

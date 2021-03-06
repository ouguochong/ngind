// Copyright 2015 The go-ethereum Authors
// This file is part of Ngin.
//
// Ngin is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Ngin is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with Ngin. If not, see <http://www.gnu.org/licenses/>.

package rpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/rs/cors"
)

const (
	maxHTTPRequestContentLength = 1024 * 128
)

// httpClient connects to a geth RPC server over HTTP.
type httpClient struct {
	endpoint   string      // HTTP-RPC server endpoint
	httpClient http.Client // reuse connection
	lastRes    []byte      // HTTP requests are synchronous, store last response
}

// Send will serialize the given msg to JSON and sends it to the RPC server.
// Since HTTP is synchronous the response is stored until Recv is called.
func (client *httpClient) Send(msg interface{}) error {
	var body []byte
	var err error

	client.lastRes = nil
	if body, err = json.Marshal(msg); err != nil {
		return err
	}

	resp, err := client.httpClient.Post(client.endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		client.lastRes, err = ioutil.ReadAll(resp.Body)
		return err
	}

	return fmt.Errorf("request failed: %s", resp.Status)
}

// Recv will try to deserialize the last received response into the given msg.
func (client *httpClient) Recv(msg interface{}) error {
	return json.Unmarshal(client.lastRes, &msg)
}

// Close is not necessary for httpClient
func (client *httpClient) Close() {
}

// SupportedModules will return the collection of offered RPC modules.
func (client *httpClient) SupportedModules() (map[string]string, error) {
	return SupportedModules(client)
}

// httpReadWriteNopCloser wraps a io.Reader and io.Writer with a NOP Close method.
type httpReadWriteNopCloser struct {
	io.Reader
	io.Writer
}

// Close does nothing and returns always nil
func (t *httpReadWriteNopCloser) Close() error {
	return nil
}

// newJSONHTTPHandler creates a HTTP handler that will parse incoming JSON requests,
// send the request to the given API provider and sends the response back to the caller.
func newJSONHTTPHandler(srv *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > maxHTTPRequestContentLength {
			http.Error(w,
				fmt.Sprintf("content length too large (%d>%d)", r.ContentLength, maxHTTPRequestContentLength),
				http.StatusRequestEntityTooLarge)
			return
		}

		w.Header().Set("content-type", "application/json")

		// create a codec that reads direct from the request body until
		// EOF and writes the response to w and order the server to process
		// a single request.
		codec := NewJSONCodec(&httpReadWriteNopCloser{r.Body, w})
		defer codec.Close()
		srv.ServeSingleRequest(codec, OptionMethodInvocation)
	}
}

// NewHTTPServer creates a new HTTP RPC server around an API provider.
func NewHTTPServer(corsString string, srv *Server) *http.Server {
	var allowedOrigins []string
	for _, domain := range strings.Split(corsString, ",") {
		allowedOrigins = append(allowedOrigins, strings.TrimSpace(domain))
	}

	c := cors.New(cors.Options{
		AllowedOrigins: allowedOrigins,
		AllowedMethods: []string{"POST", "GET"},
	})

	handler := c.Handler(newJSONHTTPHandler(srv))

	return &http.Server{
		Handler: handler,
	}
}

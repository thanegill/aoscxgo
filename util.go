package aoscxgo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// A custom error struct
type RequestError struct {
	StatusCode string

	Err error
}

// A custom error string
func (r *RequestError) Error() string {
	return fmt.Sprintf("Status : %v\nError %v", r.StatusCode, r.Err)
}

// roundTrip sends req. When no response comes back, as on a refused or
// dropped connection or a timeout, it returns the error together with a
// response that has no status code, an empty body and the error as its
// Status, so a caller that checks only the status fails with the error's
// message instead of dereferencing a nil response.
func roundTrip(transport *http.Transport, req *http.Request) (*http.Response, error) {
	res, err := transport.RoundTrip(req)
	if err != nil {
		return &http.Response{
			Status: fmt.Sprintf("no response to %s %s: %v", req.Method, req.URL.Redacted(), err),
			Header: http.Header{},
			Body:   http.NoBody,
		}, err
	}
	return res, nil
}

// delete performs DELETE to the given URL and returns the response.
func delete(client *Client, url string) *http.Response {
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("accept", "*/*")
	req.Close = false

	req.AddCookie(client.Cookie)
	res, _ := roundTrip(client.Transport, req)

	return res
}

// get performs GET to the given URL and returns the data response.
func get(client *Client, url string) (*http.Response, map[string]interface{}) {
	req, _ := http.NewRequest("GET", url, nil)

	req.Header.Set("accept", "*/*")
	req.Header.Set("x-csrf-token", client.Csrf)
	req.Close = false
	req.AddCookie(client.Cookie)
	res, _ := roundTrip(client.Transport, req)

	body := make(map[string]interface{})
	json.NewDecoder(res.Body).Decode(&body)

	return res, body
}

// get performs GET to the given URL and returns the data response.
func get_accept_text(client *Client, url string) (*http.Response, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "text/plain")
	// req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("x-csrf-token", client.Csrf)
	req.Close = false
	req.AddCookie(client.Cookie)
	return roundTrip(client.Transport, req)
}

// get performs GET to the given URL and returns the data response.
func post(client *Client, url string, json_body *bytes.Buffer) *http.Response {
	req, _ := http.NewRequest("POST", url, json_body)
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("x-csrf-token", client.Csrf)
	req.Close = false

	req.AddCookie(client.Cookie)
	res, _ := roundTrip(client.Transport, req)

	return res
}

// put performs PUT to the given URL and returns the response.
func put(client *Client, url string, json_body *bytes.Buffer) *http.Response {
	req, _ := http.NewRequest("PUT", url, json_body)
	req.Header.Set("accept", "*/*")
	req.Header.Set("x-csrf-token", client.Csrf)
	req.Close = false

	req.AddCookie(client.Cookie)
	res, _ := roundTrip(client.Transport, req)

	return res
}

// patch performs PATCH to the given URL and returns the response.
func patch(client *Client, url string, json_body *bytes.Buffer) *http.Response {
	req, _ := http.NewRequest("PATCH", url, json_body)
	req.Header.Set("accept", "*/*")
	req.Header.Set("x-csrf-token", client.Csrf)
	req.Close = false

	req.AddCookie(client.Cookie)
	res, _ := roundTrip(client.Transport, req)

	return res
}

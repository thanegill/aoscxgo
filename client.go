package aoscxgo

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	// Connection properties.
	Hostname string `json:"hostname"`
	Username string `json:"username"`
	Password string `json:"password"`
	Version  string `json:"version"`
	// Generated after Connect
	Cookie *http.Cookie `json:"cookie"`
	Csrf   string       `json:"Csrf"`
	// HTTP transport options.  Note that the VerifyCertificate setting is
	// only used if you do not specify a HTTP transport yourself.
	VerifyCertificate bool            `json:"verify_certificate"`
	Transport         *http.Transport `json:"-"`
}

// Connect creates connection to given Client object.
func Connect(c *Client) (*Client, error) {
	var err error

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: !c.VerifyCertificate},
	}

	// Always REST v10.16, the switch firmware's own: on it, v10.09 leaves out
	// attributes and map keys.
	c.Version = "v10.16"

	if c.Transport == nil {
		c.Transport = tr
	}

	cookie, csrf, err := login(c.Transport, c.Hostname, c.Version, c.Username, c.Password)

	if err != nil {
		return nil, err
	}
	c.Cookie = cookie
	c.Csrf = csrf
	return c, err
}

// Logout calls the logout endpoint to clear the session.
func (c *Client) Logout() error {
	if c == nil {
		return errors.New("nil value to Logout")
	}
	url := fmt.Sprintf("https://%s/rest/%s/logout", c.Hostname, c.Version)
	resp := logout(c.Transport, c.Cookie, c.Csrf, url)
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	return nil
}

// login performs POST to create a cookie for authentication to the given IP with the provided credentials.
func login(http_transport *http.Transport, ip string, rest_version string, username string, password string) (*http.Cookie, string, error) {
	// The credentials go in the body, never the URL: the switch logs the URLs
	// it is asked for in cleartext.
	login_url := fmt.Sprintf("https://%s/rest/%s/login", ip, rest_version)
	form := url.Values{"username": {username}, "password": {password}}
	req, _ := http.NewRequest("POST", login_url, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("accept", "*/*")
	req.Header.Set("x-use-csrf-token", "true")
	req.Close = false

	res, _ := roundTrip(http_transport, req)
	if res.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("could not log in to the switch: %s", res.Status)
	}

	csrf := res.Header.Get("X-Csrf-Token")
	cookies := res.Cookies()
	if csrf == "" || len(cookies) == 0 {
		return nil, "", errors.New("could not log in to the switch: its login answered no session cookie or CSRF token")
	}

	fmt.Println("Login Successful")

	return cookies[0], csrf, nil
}

// logout performs POST to logout using a cookie from the given URL.
func logout(http_transport *http.Transport, cookie *http.Cookie, csrf string, url string) *http.Response {
	req, _ := http.NewRequest("POST", url, nil)
	req.Header.Set("accept", "*/*")
	req.Header.Set("x-csrf-token", csrf)
	req.Close = false

	req.AddCookie(cookie)
	res, _ := roundTrip(http_transport, req)
	if res.StatusCode == http.StatusOK {
		fmt.Println("Logout Successful")
	}

	return res
}

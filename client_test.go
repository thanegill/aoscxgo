package aoscxgo

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// request is one request the fake switch was sent, as the tests compare it.
type request struct {
	Method      string
	URI         string
	ContentType string
	Body        string
}

// fakeSwitch is an HTTPS server that records every request and answers each
// with answer.
type fakeSwitch struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []request
}

func newFakeSwitch(t *testing.T, answer func(w http.ResponseWriter, r *http.Request)) *fakeSwitch {
	t.Helper()
	fake := &fakeSwitch{}
	fake.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fake.mu.Lock()
		fake.requests = append(fake.requests, request{r.Method, r.URL.RequestURI(), r.Header.Get("Content-Type"), string(body)})
		fake.mu.Unlock()
		answer(w, r)
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

// client is a logged-in client of the fake.
func (fake *fakeSwitch) client() *Client {
	return &Client{
		Hostname:  strings.TrimPrefix(fake.server.URL, "https://"),
		Version:   "v10.16",
		Cookie:    &http.Cookie{Name: "id", Value: "session"},
		Csrf:      "token",
		Transport: fake.server.Client().Transport.(*http.Transport),
	}
}

func (fake *fakeSwitch) sent() []request {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return append([]request(nil), fake.requests...)
}

func answerLogin(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "id", Value: "session"})
	w.Header().Set("X-Csrf-Token", "token")
	w.WriteHeader(http.StatusOK)
}

// sameJSON compares two JSON texts by their decoded values.
func sameJSON(t *testing.T, got string, want string) bool {
	t.Helper()
	var gotValue, wantValue interface{}
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Fatalf("not JSON: %q: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("not JSON: %q: %v", want, err)
	}
	return reflect.DeepEqual(gotValue, wantValue)
}

func TestConnectLogsInOverREST1016WithTheCredentialsInTheBody(t *testing.T) {
	password := "p&ss=w0rd?#%+ x"
	fake := newFakeSwitch(t, answerLogin)
	client, err := Connect(&Client{
		Hostname:  strings.TrimPrefix(fake.server.URL, "https://"),
		Username:  "admin",
		Password:  password,
		Transport: fake.server.Client().Transport.(*http.Transport),
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if client.Version != "v10.16" {
		t.Errorf("Version is %q, not v10.16", client.Version)
	}
	if client.Csrf != "token" || client.Cookie == nil || client.Cookie.Value != "session" {
		t.Errorf("session is %v and %q, not the login's", client.Cookie, client.Csrf)
	}
	sent := fake.sent()
	if len(sent) != 1 {
		t.Fatalf("sent %d requests, not one login: %v", len(sent), sent)
	}
	login := sent[0]
	if login.Method != "POST" || login.URI != "/rest/v10.16/login" {
		t.Errorf("logged in with %s %s, not POST /rest/v10.16/login with no query", login.Method, login.URI)
	}
	if login.ContentType != "application/x-www-form-urlencoded" {
		t.Errorf("login Content-Type is %q", login.ContentType)
	}
	form, err := url.ParseQuery(login.Body)
	if err != nil {
		t.Fatalf("login body %q: %v", login.Body, err)
	}
	want := url.Values{"username": {"admin"}, "password": {password}}
	if !reflect.DeepEqual(form, want) {
		t.Errorf("login form is %v, not %v", form, want)
	}
}

func TestConnectFailsOnARefusedLogin(t *testing.T) {
	fake := newFakeSwitch(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	client, err := Connect(&Client{Hostname: strings.TrimPrefix(fake.server.URL, "https://"), Transport: fake.server.Client().Transport.(*http.Transport)})
	if err == nil || client != nil {
		t.Fatalf("Connect answered %v, %v, not an error", client, err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error %q does not name the status", err)
	}
}

func TestConnectFailsOnALoginWithoutASession(t *testing.T) {
	fake := newFakeSwitch(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	client, err := Connect(&Client{Hostname: strings.TrimPrefix(fake.server.URL, "https://"), Transport: fake.server.Client().Transport.(*http.Transport)})
	if err == nil || client != nil {
		t.Fatalf("Connect answered %v, %v, not an error", client, err)
	}
}

// unanswered is a client of a switch that is gone: its address no longer
// takes connections.
func unanswered(t *testing.T) *Client {
	fake := newFakeSwitch(t, answerLogin)
	client := fake.client()
	fake.server.Close()
	return client
}

func TestConnectFailsWhenTheSwitchDoesNotAnswer(t *testing.T) {
	gone := unanswered(t)
	client, err := Connect(&Client{Hostname: gone.Hostname, Transport: gone.Transport})
	if err == nil || client != nil {
		t.Fatalf("Connect answered %v, %v, not an error", client, err)
	}
	if !strings.Contains(err.Error(), "no response to POST") {
		t.Errorf("error %q does not say the login went unanswered", err)
	}
}

func TestRequestsFailWhenTheSwitchDoesNotAnswer(t *testing.T) {
	client := unanswered(t)
	statusOf := func(err error) string {
		requestErr, ok := err.(*RequestError)
		if !ok {
			t.Fatalf("error %v is not a RequestError", err)
		}
		return requestErr.StatusCode
	}
	cases := []struct {
		name string
		call func() error
	}{
		{"Interface.Get", func() error { return (&Interface{Name: "1/1/1"}).Get(client) }},
		{"Interface.Update", func() error { return (&Interface{Name: "1/1/1"}).Update(client) }},
		{"Vlan.Get", func() error { return (&Vlan{VlanId: 10}).Get(client) }},
		{"Vlan.Create", func() error { return (&Vlan{VlanId: 10, Name: "ten"}).Create(client) }},
		{"L2Interface.Get", func() error { return (&L2Interface{Interface: Interface{Name: "1/1/1"}}).Get(client) }},
		{"Interface.Delete", func() error { return (&Interface{Name: "1/1/1"}).Delete(client) }},
		{"Vlan.Delete", func() error { return (&Vlan{VlanId: 10}).Delete(client) }},
	}
	for _, c := range cases {
		err := c.call()
		if err == nil {
			t.Errorf("%s succeeded with no switch", c.name)
			continue
		}
		if status := statusOf(err); !strings.Contains(status, "no response to ") {
			t.Errorf("%s failed with %q, not the unanswered request", c.name, status)
		}
	}
	if err := client.Logout(); err == nil || !strings.Contains(err.Error(), "no response to POST") {
		t.Errorf("Logout answered %v, not the unanswered request", err)
	}
}

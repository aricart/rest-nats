package httpnatsbridge

import (
	"fmt"
	"github.com/nats-io/go-nats"
	"net/http"
	"strings"
	"testing"
	"time"
)

func natsURL(rns *Server) string {
	return fmt.Sprintf("nats://%s", rns.NatsHostPort)
}

func httpURL(rns *Server) string {
	return fmt.Sprintf("http://%s", rns.HttpHostPort)

}

func TestBasics(t *testing.T) {
	rns := NewRnServer(nil)
	rns.Start()

	nc, err := nats.Connect(natsURL(rns))
	if err != nil {
		t.Fatalf("failed to connect to: %v", err)
	}
	done := make(chan string)
	sub, err := nc.Subscribe(">", func(msg *nats.Msg) {
		if msg.Subject != "hello.world" {
			t.Fatalf("didn't get expected subject: %v", msg.Subject)
		}
		if string(msg.Data) != "hi!" {
			t.Fatalf("didn't get expected payload: %v", string(msg.Data))
		}
		done <- "httpDone"
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	_, err = http.Post(httpURL(rns)+"/hello/world", "text/plain", strings.NewReader("hi!"))
	if err != nil {
		t.Fatalf("error posting: %v", err)
	}

	<-done

	sub.Unsubscribe()

	nc.Close()
	rns.Stop()
	time.Sleep(time.Second * 1)

	_, err = nats.Connect(natsURL(rns))
	if err == nil {
		t.Fatalf("failed to stop: %v", err)
	}

}

func TestPathToSubject(t *testing.T) {
	type io struct {
		p string
		s string
	}
	a := []io{
		{p: "/", s: ""},
		{p: "/foo", s: "foo"},
		{p: "/foo", s: "foo"},
		{p: "/foo/", s: "foo"},
		{p: "/foo/foo", s: "foo.foo"},
	}

	for _, tc := range a {
		v := PathToSubject(tc.p)
		if v != tc.s {
			t.Errorf("expected '%s' to become '%s' but got '%s'", tc.p, tc.s, v)
		}
	}
}

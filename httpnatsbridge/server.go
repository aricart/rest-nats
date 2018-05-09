package httpnatsbridge

import (
	"fmt"
	"github.com/nats-io/gnatsd/server"
	"github.com/nats-io/go-nats"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"log"
)

type ServerOptions struct {
	Embed        bool
	NatsHostPort string
	HttpHostPort string
	MonPort      int
}

func DefaultServerOptions() *ServerOptions {
	opts := ServerOptions{}
	opts.Embed = true
	opts.NatsHostPort = "localhost:0"
	opts.HttpHostPort = "localhost:0"
	opts.MonPort = 6619
	return &opts
}

type Server struct {
	ServerOptions
	gnatsd     *server.Server
	httpServer *http.Server
	nc         *nats.Conn
	listener   net.Listener
	msgQueue   chan *nats.Msg
	httpDone   chan string
}

func NewRnServer(options *ServerOptions) *Server {
	if options == nil {
		options = DefaultServerOptions()
	}
	v := Server{}
	v.ServerOptions = *options
	return &v
}

func (s *Server) GetOptions() *ServerOptions {
	v := DefaultServerOptions()
	v.Embed = s.Embed
	v.NatsHostPort = s.NatsHostPort
	v.HttpHostPort = s.HttpHostPort
	return v
}

func (s *Server) isEmbedded() bool {
	return s.Embed
}

func (s *Server) Start() {
	s.handleSignals()
	s.httpDone = make(chan string)
	s.maybeStartGnatsd()
	s.startNatsConnection()

	s.msgQueue = make(chan *nats.Msg, 1000)
	go s.dispatch()

	s.startHttp()
}

func (s *Server) GetEmbeddedPort() int {
	return s.gnatsd.Addr().(*net.TCPAddr).Port
}

// handle signals so we can orderly shutdown
func (s *Server) handleSignals() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT)
	go func() {
		for sig := range c {
			switch sig {
			case syscall.SIGINT:
				s.Stop()
				os.Exit(0)
			}
		}
	}()
}

// Stop the server
func (s *Server) Stop() {
	// stop the http listener
	s.httpServer.Shutdown(nil)

	// stop message processing
	s.msgQueue <- nil

	// wait for all messages to be dispatched
	<-s.httpDone

	// close nats connection
	s.nc.Close()
	s.nc = nil

	// stop gnatsd
	if s.gnatsd != nil {
		fmt.Println("Stopping embedded gnatsd")
		s.gnatsd.Shutdown()
	}

	fmt.Printf("max messages queued %d\n", maxQueue)
}

type HostPort struct {
	host string
	port int
}

func (h *HostPort) String() string {
	return net.JoinHostPort(h.host, strconv.Itoa(h.port))
}

func NewHostPort(hostPort string) (HostPort, error) {
	var err error
	var sport string
	hp := HostPort{}
	hp.host, sport, err = net.SplitHostPort(hostPort)
	p, err := strconv.Atoi(sport)
	if err != nil {
		return hp, err
	}
	hp.port = p
	return hp, nil
}

func (s *Server) maybeStartGnatsd() {
	if s.isEmbedded() {
		fmt.Println("Starting gnatsd")
		nhp, err := NewHostPort(s.NatsHostPort)
		if err == nil {
			opts := server.Options{
				Host:           "localhost",
				Port:           nhp.port,
				NoLog:          false,
				NoSigs:         true,
				MaxControlLine: 1024,
			}
			s.gnatsd = server.New(&opts)
			if s.gnatsd == nil {
				panic("unable to create gnatsd")
			}

			go s.gnatsd.Start()

			if s.isEmbedded() && !s.gnatsd.ReadyForConnections(5*time.Second) {
				panic("unable to start embedded server")
			}

			if nhp.port < 1 {
				nhp.port = s.GetEmbeddedPort()
				s.NatsHostPort = nhp.String()
			}

			fmt.Printf("Embedded NATS server started [%v]\n", nhp.String())
		} else {
			panic(err)
		}
	}
}

func (s *Server) startNatsConnection() {
	var err error
	url := fmt.Sprintf("nats://%s", s.NatsHostPort)
	s.nc, err = nats.Connect(url)
	if err != nil {
		panic(fmt.Sprintf("unable to connect to server [%s]: %v", url, err))
	}
	fmt.Printf("Connected [%s]\n", url)
}

// convert the REST request path to a NATS subject
func PathToSubject(u string) string {
	a := strings.Split(u, "/")
	chunks := make([]string, 0, len(a))
	for _, s := range a {
		if s != "" {
			chunks = append(chunks, s)
		}
	}
	return strings.Join(chunks, ".")
}

var maxQueue int64

func (s *Server) dispatch() {
	for {
		select {
		case msg := <-s.msgQueue:
			c := int64(len(s.msgQueue))
			if c > maxQueue {
				maxQueue = c
			}
			// if nil, the server is shutting down
			if msg != nil {
				s.nc.Publish(msg.Subject, msg.Data)
			} else {
				// unblock Shutdown()
				s.httpDone <- "done"
				break
			}
		}
	}
}

func (s *Server) startHttp() error {
	// start listening
	var err error
	s.listener, err = net.Listen("tcp", s.HttpHostPort)
	if err != nil {
		panic(fmt.Errorf("[ERR] cannot listen for http requests: %v", err))
	}
	// if the port was auto selected, update the config
	hp, err := NewHostPort(s.HttpHostPort)
	if hp.port < 1 {
		if err != nil {
			panic(err)
		}
		hp.port = s.listener.Addr().(*net.TCPAddr).Port
		s.HttpHostPort = hp.String()
	}
	fmt.Printf("[INF] http server started [%v]\n", hp.String())



	s.httpServer = &http.Server{
		ReadTimeout: 5*time.Second,
		WriteTimeout:10*time.Second,
		ErrorLog: log.New(os.Stderr, "", log.LstdFlags),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// fast error - we cannot generate a subject if posted to '/'
			url := r.URL.String()
			if url == "/" {
				w.WriteHeader(http.StatusBadRequest)
				return
			} else {
				// create a nats msg and stuff the data
				msg := &nats.Msg{}
				msg.Subject = PathToSubject(url)
				var bytes []byte
				var err error
				if r.ContentLength > 0 {
					bytes, err = ioutil.ReadAll(r.Body)
					if err != nil {
						fmt.Printf("[ERR] reading [%s] request: %v\n", r.RemoteAddr, err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					r.Body.Close()
					msg.Data = bytes
				}
				// push it to a channel
				s.msgQueue <- msg

				// ok the client
				w.WriteHeader(http.StatusOK)
			}
		}),
	}

	go func() {
		if err := s.httpServer.Serve(s.listener); err != nil {
			// we orderly shutdown the server?
			if !strings.Contains(err.Error(), "http: Server closed") {
				panic(fmt.Errorf("[ERR] http server: %v\n", err))
			}
		}
		s.httpServer.Handler = nil
		s.httpServer = nil
		fmt.Printf("HTTP server has stopped\n")
	}()

	return nil
}

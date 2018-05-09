package main

import (
	"flag"
	"github.com/aricart/rest-nats/httpnatsbridge"
	"runtime"
)

func main() {
	opts := httpnatsbridge.DefaultServerOptions()

	flag.BoolVar(&opts.Embed, "e", false, "Embed gnatsd")
	flag.StringVar(&opts.NatsHostPort, "hp", "localhost:4222", "NATS host port")
	flag.StringVar(&opts.HttpHostPort, "w", "localhost:8080", "HTTP host port")
	flag.Parse()

	//go func() {
	//	http.ListenAndServe(fmt.Sprintf(":%d", opts.MonPort), http.DefaultServeMux)
	//}()
	//fmt.Printf("Monitoring http://localhost:%d/debug/vars\n", opts.MonPort)

	server := httpnatsbridge.NewRnServer(opts)
	server.Start()

	runtime.Goexit()
}

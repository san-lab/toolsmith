package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/san-lab/toolsmith/httphandler"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"strconv"
)

//Parsing flags "ethport" and "host"
//Initializing EthRPC client
//TODO: Take care of the favicon.ico location
func main() {

	ethRPCAddress := flag.String("ethRPCAddress", "localhost:8545", "default RPC access point")
	httpPortF := flag.String("httpPort", "8090", "http port")
	mockMode := flag.Bool("mockMode", false, "should mock http RPC client")
	dumpRPC := flag.Bool("dumpRPC", false, "should dump RPC responses to files")
	startWatchdog := flag.Bool("startWatchdog", false, "should a watchdog  be started")
	withBasicAuth := flag.Bool("withAuth", true, "should Basic Authentication be enabled")
	httpsPortF := flag.Int("httpsPort", 0, "https port. tls not started if not provided. requires server.crt & server.key")
	flag.Parse()

	c := httphandler.Config{}
	c.EthHost = *ethRPCAddress
	httpPort := *httpPortF
	httpsPort := *httpsPortF
	c.MockMode = *mockMode
	c.DumpRPC = *dumpRPC
	c.StartWatchdog = *startWatchdog
	c.BasicAuth = *withBasicAuth
	fmt.Println("Here")

	interruptChan := make(chan os.Signal)
	wg := &sync.WaitGroup{}
	signal.Notify(interruptChan, os.Interrupt)
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	ctx = context.WithValue(ctx, "WaitGroup", wg)
	handler, err := httphandler.NewHttpHandler(c, ctx)
	if err != nil {
		panic(err)
	}
	http.HandleFunc("/favicon.ico", favIconHandler)
	//Beware! This config means that all the static images - also the ones called from the templates -
	// have to be addressed as "/static/*", regardless of the location of the template
	fs := http.FileServer(http.Dir("static"))
	http.HandleFunc("/static/", http.StripPrefix("/static", fs).ServeHTTP)
	http.HandleFunc("/", handler.GetHandler(*withBasicAuth))
	srv := http.Server{Addr: ":" + httpPort}
	var tlsSrv http.Server
	if httpsPort > 0 {
		tlsSrv = http.Server{Addr:":" + strconv.Itoa(httpsPort) }
	}
	go func() {
		select {
		case <-interruptChan:
			cancel()
			srv.Shutdown(context.TODO())
			if httpsPort > 0 {
				tlsSrv.Shutdown(context.TODO())
			}
			return
		}
	}()
	if httpsPort > 0 {
		go func() {
			log.Println(tlsSrv.ListenAndServeTLS("server.crt", "server.key"  ))
		}()
	}
	log.Println(srv.ListenAndServe())
	wg.Wait()
}

func favIconHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/build_32x32.png")
}

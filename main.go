package main

import (
	"flag"
	"fmt"
	"github.com/san-lab/toolsmith/httphandler"
	"net/http"
	"os"
	"os/signal"
	"context"
	"sync"
	"log"
)

//Parsing flags "ethport" and "host"
//Initializing EthRPC client
//TODO: Take care of the favicon.ico location
func main() {

	ethRPCAddress := flag.String("ethRPCAddress", "localhost:8545", "default RPC access point")
	httpPort := flag.String("httpPort", "8090", "http port")
	mockMode := flag.Bool("mockMode", false, "should mock http RPC client")
	dumpRPC := flag.Bool("dumpRPC", false, "should dump RPC responses to files")
	startWatchdog := flag.Bool("startWatchdog", false, "if a blochchain network watchdog should be started")
	flag.Parse()

	c := httphandler.Config{}
	c.EthHost = *ethRPCAddress
	c.HttpPort = *httpPort
	c.MockMode = *mockMode
	c.DumpRPC = *dumpRPC
	c.StartWatchdog = *startWatchdog
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
	http.HandleFunc("/", handler.Handler)
	srv := http.Server{Addr:":"+c.HttpPort}
	go func() {
		select {
		case <-interruptChan:
			cancel()
			srv.Shutdown(context.TODO())
			return
		}
	}()
	log.Println(srv.ListenAndServe())
	wg.Wait()
}

func favIconHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/build_32x32.png")
}

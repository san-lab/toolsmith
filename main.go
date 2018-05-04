package main

import (
	"flag"
	"fmt"
	"github.com/san-lab/toolsmith/httphandler"
	"log"
	"net/http"
)

//Parsing flags "ethport" and "host"
//Initializing EthRPC client
//TODO: Take care of the favicon.ico location
func main() {

	ethRPCAddress := flag.String("ethRPCAddress", "localhost:8545", "default RPC access point")
	httpPort := flag.String("httpPort", "8090", "http port")
	mockMode := flag.Bool("mockMode", false, "should mock http RPC client")
	dumpRPC := flag.Bool("dumpRPC", false, "should dump RPC responses to files")
	flag.Parse()
	c := httphandler.Config{}

	c.EthHost = *ethRPCAddress
	c.HttpPort = *httpPort
	c.MockMode = *mockMode
	c.DumpRPC = *dumpRPC
	fmt.Println("Here")
	handler, err := httphandler.NewHttpHandler(c)

	if err != nil {
		panic(err)
	}
	http.HandleFunc("/favicon.ico", favIconHandler)
	//Beware! This config means that all the static images - also the ones called from the templates -
	// have to be addressed as "/static/*", regardless of the location of the template
	fs := http.FileServer(http.Dir("static"))
	http.HandleFunc("/static/", http.StripPrefix("/static", fs).ServeHTTP)
	http.HandleFunc("/", handler.Handler)
	log.Fatal(http.ListenAndServe(":"+c.HttpPort, nil))
}

func favIconHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/build_32x32.png")
}

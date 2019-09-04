package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

func copyHeaders(src, dst *http.Header) {
	for k, vv := range *src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

type proxy struct {
	chost       string
	cport       string
	lport       string
	headersonly bool
	mockOptions bool
}

func (p *proxy) ServeHTTP(wr http.ResponseWriter, req *http.Request) {

	defer req.Body.Close()

	bbuf, err := ioutil.ReadAll(req.Body)
	if err != nil {
		fmt.Fprintln(wr, err)
		return

	}
	//Dump request:
	fmt.Println("Request-----------------")
	fmt.Println("From: ", req.RemoteAddr)
	fmt.Println(req.Method, " ", req.URL)
	for k, vv := range req.Header {
		for _, v := range vv {
			fmt.Println(k, " ", v)
		}
	}
	fmt.Println(string(bbuf))

	// Now check if to mock the response to the OPTIONS call
	if p.mockOptions {

		if req.Method == http.MethodOptions {
			fmt.Println("Mocking the http OPTIONS call")
			wr.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTION")
			wr.Header().Set("Access-Control-Allow-Headers", "Content-type")
			wr.Header().Set("Content-type", "application/json")
			wr.Header().Set("Access-Control-Allow-Origin", "*")
			wr.WriteHeader(http.StatusOK)
			return


		}





	}


	client := &http.Client{}
	//http: Request.RequestURI can't be set in client requests.
	//http://golang.org/src/pkg/net/http/client.go
	//newUrl := "http://" + strings.Replace(req.Host, ":"+ p.lport, ":"+ p.cport,1) + req.RequestURI
	newUrl := "http://" + p.chost + ":" + p.cport + req.RequestURI
	newReq, err := http.NewRequest(req.Method, newUrl, bytes.NewReader(bbuf))
	if err != nil {
		http.Error(wr, fmt.Sprint(err), http.StatusBadGateway)
		return
	}

	copyHeaders(&req.Header, &newReq.Header)
	resp, err := client.Do(newReq)

	if err != nil {
		http.Error(wr, fmt.Sprint(err), http.StatusInternalServerError)
		log.Fatal("ServeHTTP:", err)
	}

	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(wr, "%s", err)
	}

	fmt.Println("Response------------------")
	fmt.Println(resp.Status)

	for k, vv := range resp.Header {
		for _, v := range vv {
			fmt.Println(k, " ", v)
		}
	}
	if !p.headersonly {
		fmt.Println(string(respBytes))
	}

	destHeader := wr.Header()
	copyHeaders(&resp.Header, &destHeader)

	wr.WriteHeader(resp.StatusCode)
	wr.Write(respBytes)
}

func main() {
	lhost := flag.String("listeningHost", "localhost", "The listening address. It should be localhost normally")
	lport := flag.String("listenPort", "8080", "The listening port")
	chost := flag.String("callHost", "localhost", "The server to call")
	cport := flag.String("callPort", "9090", "The port to call")
	honly := flag.Bool("headersOnly", false, "If true no message body dump")
	mockOptions := flag.Bool("mockOptions", false, "Should the http OPTION method be mocked")
	tls := flag.Bool("tls", false, "TLS enabled")
	flag.Parse()

	handler := &proxy{}
	handler.cport = *cport
	handler.lport = *lport
	handler.chost = *chost
	handler.headersonly = *honly
	handler.mockOptions = *mockOptions
	host := *lhost + ":" + *lport
	log.Println("Starting proxy server on", host)
	if *tls {
		if err := http.ListenAndServeTLS("0.0.0.0:"+*lport, "server.crt", "server.key", handler); err != nil {
			log.Fatal("ListenAndServeTLS:", err)
		}
	} else {
		if err := http.ListenAndServe(host, handler); err != nil {
			log.Fatal("ListenAndServe:", err)
		}
	}
}

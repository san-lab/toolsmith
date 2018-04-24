package client

import (
	"io/ioutil"
	"log"
	"strings"
	"net/http"
	"net"
	"encoding/json"
	"bytes"
)

type MockClient struct {
	knownResponses map[string][]byte

}

func NewMockClient() *MockClient {
	m := &MockClient{map[string][]byte{}}
	m.LoadMocks()
	return m
}


func (m *MockClient) LoadMocks() error {

	//Try to scan the mockjson subdirectory
	files, err := ioutil.ReadDir("./client/mockjson")
	if err != nil {
		log.Println(err)
		return err
	}

	for _,file := range files {
		name := file.Name()
		if ! (strings.Index(name, ".json") > 0) {
			continue
		}
		log.Println(name)
		buff, _ := ioutil.ReadFile("./client/mockjson/" + name)
		m.knownResponses[strings.TrimSuffix(name, ".json")] = buff

	}

	//err = json.Unmarshal(raw, &parr )
	//if err != nil {

 return nil
}


func (m *MockClient) Do(req *http.Request) (*http.Response, error) {
	res := &http.Response{}
	key , _, err := net.SplitHostPort(req.URL.Host)
	if err != nil {
		log.Println(err)
	}
	log.Println(key)
	defer req.Body.Close()
	rbytes, err := ioutil.ReadAll(req.Body)
	if err != nil {
		log.Println(err)
	}
	ec := EthCommand{}
	err = json.Unmarshal(rbytes, &ec)
	if err != nil {
		log.Println(err)
	}
	key = key +"_"+ec.Method
	log.Println(key)
	buf, ok := m.knownResponses[key]

	if ok {
		res.Body = myBody{bytes.NewReader(buf)}
		res.StatusCode=200
		res.Status="200 Mockup successful"
	} else {
		res.StatusCode = 404
		res.Status = "No mockup for "+key
		res.Body = myBody{ bytes.NewReader([]byte{})}
	}
	return res, nil
}


type myBody struct {
	reader *bytes.Reader
}
func (mb myBody) Read(bb []byte) (int, error) {
	return mb.reader.Read(bb)
}
func (mb myBody) Close() error {
	return nil
}
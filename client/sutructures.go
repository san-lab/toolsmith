package client

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

//This file contains Go structures and methods needed for json marshal/unmarshal

//Two-step json unmarshalling. First from reader into CallData.Response, the "result" stays as 'rawMessage'.
//Then - if the result is a known structure - it is decoded into CallData.Result.ParsedResult
func Decode(reader io.Reader, data *CallData) error {
	err := json.NewDecoder(reader).Decode(&data.Response)
	if err != nil {
		return err
	}
	if data.Response.Error != nil {
		return nil // it is an error, but not the Client's error
	}
	var p ResultType
	switch data.Command.Method {
	case "admin_datadir": // All the single-strings results fall here
		s := StringResult("")
		p = &s
	case "admin_peers":
		p = PeerArray(make([]PeerInfo, 0, 10))

	case "eth_blockNumber":
		b := IntegerResult(0)
		p = &b
	case "admin_nodeInfo":
		p = &NodeInfo{}
	}
	if p != nil {
		err = p.parse(data)
	}
	if err != nil {
		fmt.Println(err)
	}

	return err
}

//An interface to make Decode() bearable.
//Known ResultTypes must implement the parse() method
type ResultType interface {
	parse(*CallData) error
}

//This is inlined from github.com/ethereum/go-ethereum/p2p/server.go
//This is the struct used by the gethrpc admin api
//
// NodeInfo represents a short summary of the information known about the host.
type NodeInfo struct {
	ID    string `json:"id"`    // Unique node identifier (also the encryption key)
	Name  string `json:"name"`  // Name of the node, including client type, version, OS, custom data
	Enode string `json:"enode"` // Enode URL for adding this peer from remote Peers
	IP    string `json:"ip"`    // IP address of the node
	Ports struct {
		Discovery int `json:"discovery"` // UDP listening port for discovery protocol
		Listener  int `json:"listener"`  // TCP listening port for RLPx
	} `json:"ports"`
	ListenAddr string                 `json:"listenAddr"`
	Protocols  map[string]interface{} `json:"protocols"`
}

func (ni *NodeInfo) parse(data *CallData) error {
	err := json.Unmarshal(data.Response.Result, &ni)
	if err != nil {
		return err
	}
	data.Parsed = true
	data.ParsedResult = ni
	return err
}

// This structure is inlined from github.com/ethereum/go-ethereum/p2p/peer
// I inline it to avoid dependencies, but at a price
// PeerInfo represents a short summary of the information known about a connected
// peer. Sub-protocol independent fields are contained and initialized here, with
// protocol specifics delegated to all connected sub-protocols.
type PeerInfo struct {
	ID      string   `json:"id"`   // Unique node identifier (also the encryption key)
	Name    string   `json:"name"` // Name of the node, including client type, version, OS, custom data
	Caps    []string `json:"caps"` // Sum-protocols advertised by this particular peer
	Network struct {
		LocalAddress  string `json:"localAddress"`  // Local endpoint of the TCP data connection
		RemoteAddress string `json:"remoteAddress"` // Remote endpoint of the TCP data connection
	} `json:"network"`
	Protocols map[string]interface{} `json:"protocols"` // Sub-protocol specific metadata fields
}

//Truncating the port number. This is needed so many times that the method is justified
func (p PeerInfo) RemoteHostMachine() string {
	return p.Network.RemoteAddress[:strings.Index(p.Network.RemoteAddress, ":")]
}

//A type to hook the "parse()" method on. *This* is a ResultType.
type PeerArray []PeerInfo

func (pa PeerArray) parse(data *CallData) error { // Hope this works - an array is already a pointer...
	//p := make([]PeerInfo,0,10)
	err := json.Unmarshal(data.Response.Result, &pa)
	if err != nil {
		return err
	}
	data.Parsed = true
	data.ParsedResult = pa
	return err
}

//A ResultType for eth_blockNumber
type IntegerResult int32

//And the required method
func (bn *IntegerResult) parse(data *CallData) error {
	var tmp string
	var bn2 int64
	err := json.Unmarshal(data.Response.Result, &tmp)
	if err != nil {
		return err
	}
	bn2, err = strconv.ParseInt(tmp, 0, 32)
	if err != nil {
		return err
	}
	data.ParsedResult = IntegerResult(bn2)
	data.Parsed = true
	return err
}

type StringResult string

func (dd *StringResult) parse(data *CallData) error {
	err := json.Unmarshal(data.Response.Result, &dd)
	if err != nil {
		return err
	}
	data.ParsedResult = dd
	data.Parsed = true
	return nil
}

//Hijacked from https://github.com/onrik/ethrpc/blob/master/ethrpc.go
type EthResponse struct {
	ID      int             `json:"id"`
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *EthError       `json:"error"`
}

// EthError - ethereum error
type EthError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type EthCommand struct {
	Jsonrpc string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	Id      uint          `json:"id"`
}

//A wrapper to pass around the call Context, Command, Result, and Error (if any)
type CallData struct {
	Context      CallContext
	Command      EthCommand
	Response     EthResponse
	Parsed       bool // if the "result" has been decoded to a specific structure
	ParsedResult interface{}
	Node         string
	RandomStuff  map[string]interface{} //Ugh this is ugly. But still learning templates
}

//TODO: expand this stub
type CallContext struct {
	ClientHostName string
	ClientIp       string
	TargetNode     string
	RawMode        bool
	RequestPath    string
}

package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
	"time"
)

//This file contains Go structures and methods needed for json marshal/unmarshal

//Two-step json unmarshalling. First from reader into CallData.Response, the "result" stays as 'rawMessage'.
//Then - if the result is a known structure - it is decoded into CallData.Result.ParsedResult
func Decode(reader io.Reader, data *CallData) error {
	respBytes, err := ioutil.ReadAll(reader)
	if err != nil {
		return err
	}

	//if data.RawJson {
	jcom, err := json.Marshal(data.Command)
	if err != nil {
		fmt.Println(err) //Not sure if one should do more - this is Marshalling our own command...

	}
	data.JsonRequest = string(jcom)
	var buf bytes.Buffer
	err = json.Indent(&buf, respBytes, "", " ")
	data.JsonResponse = buf.String()
	//return err
	//}

	//err := json.NewDecoder(reader).Decode(&data.Response)
	err = json.Unmarshal(respBytes, &data.Response)
	if err != nil {
		return err
	}

	if data.Response.Error != nil {
		return nil // it is an error, but not the Client's error
	}

	var p interface{}
	switch data.Command.Method {
	case "admin_datadir", "net_version": // All the single-strings results fall here
		s := StringResult("")
		p = &s
	case "admin_peers":
		p = &PeerArray{}

	case "eth_blockNumber": //Result is not a struct, just an 0xdddd string representing a number
		//b := HexString(0)
		//p = &b
		p = &BlockNumberSample{}
	case "admin_nodeInfo":
		p = &NodeInfo{}
	case "txpool_status":
		p = &TxpoolStatusSample{}
	}
	if p != nil {
		err = json.Unmarshal(data.Response.Result, p)
		if err == nil {
			data.Parsed = true
			data.ParsedResult = p
			if s, ok := p.(stampable); ok {
				s.stamp()
			}
		}
	}
	if err != nil {
		fmt.Println(err)
	}
	return err
}

//Just playing with the idea of timestamping the measurements
type stampable interface {
	stamp()
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
	return json.Unmarshal(data.Response.Result, ni)
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

//A type to hook the "parse()" method on. *This* is a ParseableResultType.
type PeerArray []PeerInfo

//If the json "Result" is just a string
type StringResult string

//txpool_status structure - nothing appropriate found in geth :-(
type TxpoolStatusSample struct {
	Pending HexString `json:"pending",string`
	Queued  HexString `json:"queued",string`
	Sampled MyTime    `json:"-"`
}

func (txs *TxpoolStatusSample) stamp() {
	txs.Sampled = MyTime(time.Now())
}

type HexString int64

func (h *HexString) UnmarshalText(text []byte) (err error) {
	var tmpI int64
	tmpI, err = strconv.ParseInt(string(text), 0, 64)
	*h = HexString(tmpI)
	return err
}

//As the name says it
type BlockNumberSample struct {
	BlockNumber HexString
	Sampled     MyTime
}

//The RPC call returns just an 0x123 string...
func (bns *BlockNumberSample) UnmarshalJSON(raw []byte) error {
	bns.BlockNumber = HexString(0)
	return json.Unmarshal(raw, &bns.BlockNumber)
}
func (bns *BlockNumberSample) stamp() {
	bns.Sampled = MyTime(time.Now())
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

//A wrapper to pass around the call Context, RPC, Result, and Error (if any)
type CallData struct {
	Context      CallContext
	Command      EthCommand
	Response     EthResponse
	Parsed       bool // if the "result" has been decoded to a specific structure
	ParsedResult interface{}
	NodeAddress  string
	NodeRPCport  string
	RandomStuff  map[string]interface{} //Ugh this is ugly. But still learning templates
	RawJson      bool                   //How to parse
	JsonRequest  string
	JsonResponse string
}

//TODO: expand this stub
type CallContext struct {
	ClientHostName string
	ClientIp       string
	TargetNode     string
	RawMode        bool
	RequestPath    string
}

package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/san-lab/toolsmith/templates"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"fmt"
)

//A rest api client, wrapping an http client
//The struct also contains a map of addresses of known nodes' end-points
//The field Port - to memorize the default Port (a bit of a stretch)
type Client struct {
	DefaultEthNode string
	UserAgent      string
	Port           string
	httpClient     *http.Client
	nodes          map[string]bool
	seq            uint
	r              *templates.Renderer
	LocalInfo      CallContext
	NetModel       BlockchainNet //TODO oe should it be vice-versa?

}

const defaultTimeout = 3 * time.Second

//Creates a new rest api client
//If something like ("www.node:8666",8545) is passed, an error is thrown
func NewClient(ethHost string, port string) (c *Client, err error) {
	c = &Client{httpClient: http.DefaultClient}
	c.DefaultEthNode = ethHost
	if strings.Contains(ethHost, ":") {
		c.Port = strings.Split(ethHost, ":")[1]
	} else {
		c.Port = port
	}
	c.nodes = make(map[string]bool)
	c.seq = 0
	c.httpClient.Timeout = defaultTimeout
	c.r = templates.NewRenderer()
	//TODO handle error
	c.LocalInfo, _ = GetLocalInfo()
	c.NetModel = *NewBlockchainNet()

	return
}

//Just a sequence to number the rest calls (the "id" field)
//TODO: wrap the sequence as a in a Type
func (c *Client) nextID() (id uint) {
	id = c.seq
	c.seq++
	return
}

//Generic call to the ethereum api's. Uses structures corresponding to the api json specs
//The response gets enclosed in the CallData argument
func (c *Client) ActualRpcCall(data *CallData) error {
	data.Command.Id = c.nextID()
	jcom, _ := json.Marshal(data.Command)
	//TODO: allow to define and memorize node-specific ports
	host := "http://" + data.Context.TargetNode
	if !strings.Contains(data.Context.TargetNode, ":") {
		host = host + ":" + c.Port
	}
	req, err := http.NewRequest("POST", host, bytes.NewReader(jcom))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.httpClient.Do(req)

	if err != nil {
		fmt.Println(err)
		c.NetModel.Unreachables[host] = MyTime(time.Now())
		return err
	}
	defer resp.Body.Close()
	err = Decode(resp.Body, data)
	return err
}

//Scan the network following successive peer lists.
//If rebiuld == true discard the old network model
func (client *Client) ScanNertwork(rebuild bool) error {
	if rebuild {
		client.NetModel.ReachableNodes = map[string]*Node{}
	}
	err := client.CollectNodeInfoRecursively(client.DefaultEthNode, client.Port)
	if err != nil {
		return err
	}

	for _, node := range client.NetModel.ReachableNodes {
		data := client.NewCallData("eth_blockNumber")
		//TODO: preferred address
		var err error
		for target := range node.KnownAddresses {
			data.Context.TargetNode = target
			err = client.ActualRpcCall(data)
			if err == nil {
				break
			}
		}
		if err != nil {

			return err
		}
		node.LastBlockNumberQuery = BlockNumberQuery{int32(data.ParsedResult.(IntegerResult)), MyTime(time.Now())}
	}

	return nil
}

//Collect a single node Info, including Peers
//This method will not update the NetworkModek
func (cl *Client) CollectNodeInfo(address string, port string) (node *Node, err error) {
	callData := cl.NewCallData("admin_nodeInfo")
	callData.Context.TargetNode = address
	//TODO this is overwritting. The whole RPC port handling due for refactoring
	cl.Port = port
	err = cl.ActualRpcCall(callData)
	if err != nil {
		return nil, err
	}

	ni, ok := callData.ParsedResult.(*NodeInfo)
	if !ok {
		fmt.Printf("expected %T got %T", ni, callData.ParsedResult)
		return nil, errors.New("could not parse the NodeInfo")
	}

	node = NewNode()
	node.ID = ni.ID
	node.Name = ni.Name
	node.ThisNodeInfo = *ni
	node.RPCPort = port
	node.Status = Active
	node.KnownAddresses[address] = true

	callData = cl.NewCallData("admin_peers")
	callData.Context.TargetNode = address
	err = cl.ActualRpcCall(callData)
	if err != nil {
		return node, err //Partially successful method call
	}
	node.Peers, ok = callData.ParsedResult.(PeerArray)
	if !ok {
		return node, errors.New("Could not parse the result of Peers of" + address)
	}
	return node, nil
}

//Collect  nodes Info recursing through peers
//The effects are in the NetworkModel
func (cl *Client) CollectNodeInfoRecursively(address string, port string) error {
	newnode, err := cl.CollectNodeInfo(address, port)
	if err != nil {
		return err
	}
	if knownnode, ok := cl.NetModel.ReachableNodes[newnode.ID]; ok {
		//TODO collect addresses
		knownnode.KnownAddresses[address] = true
		fmt.Printf("Found aganin %s\n", knownnode.ID)
	} else { //a new guy!
		cl.NetModel.ReachableNodes[newnode.ID] = newnode
		for _, peer := range newnode.Peers {
			err := cl.CollectNodeInfoRecursively(peer.RemoteHostMachine(), port)
			if err != nil {
				//cl.NetModel.Unreachables[peer.RemoteHostMachine()] = MyTime(time.Now())
				//fmt.Println(err)
			}

		}
	}
	return nil
}

//The name says it.
// "method" name is needed for constructing the Command field
//     - which is complete and only the "ID" integer is meant to be changed
func (c *Client) NewCallData(method string) *CallData {
	com := EthCommand{"2.0", method, []interface{}{}, 0}
	ctx := c.LocalInfo // Cloning. This at least is my intention ;-)
	calldata := &CallData{Context: ctx, Command: com, Response: EthResponse{}, RandomStuff: map[string]interface{}{}}
	return calldata
}

//Just a stub of a function gathering host system info
func GetLocalInfo() (CallContext, error) {
	hostname, err := os.Hostname()
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {

	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	ipaddress := localAddr.IP.String()
	return CallContext{ClientHostName: hostname, ClientIp: ipaddress}, err
}

func CamelCaseKnownCommand(command *string) bool {
	for _, cc := range KnownEthCommands {
		if strings.EqualFold(*command, cc) {
			*command = cc
			return true
		}
	}
	return false
}

var KnownEthCommands = []string{"admin_addPeer", "debug_backtraceAt", "miner_setExtra", "personal_ecRecover", "txpool_content",
	"admin_datadir", "debug_blockProfile", "miner_setGasPrice", "personal_importRawKey", "txpool_inspect",
	"admin_nodeInfo", "debug_cpuProfile", "miner_start", "personal_listAccounts", "txpool_status",
	"admin_peers", "debug_dumpBlock", "miner_stop", "personal_lockAccount",
	"admin_setSolc", "debug_gcStats", "miner_getHashrate", "personal_newAccount",
	"admin_startRPC", "debug_getBlockRlp", "miner_aetEtherbase", "personal_unlockAccount",
	"admin_startWS", "debug_goTrace", "personal_sendTransaction",
	"admin_stopRPC", "debug_memStats", "personal_sign",
	"admin_stopWS", "debug_seedHashsign"}

package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/san-lab/toolsmith/templates"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

//A rest api client, wrapping an http client
//The struct also contains a map of addresses of known nodes' end-points
//The field Port - to memorize the default Port (a bit of a stretch)
type Client struct {
	DefaultEthNode       string
	UserAgent            string
	httpClient           HttpClient
	seq                  uint
	r                    *templates.Renderer
	LocalInfo            CallContext
	NetModel             BlockchainNet
	DefaultRPCPort       string
	DebugMode            bool
	UnreachableAddresses map[string]MyTime
	VisitedNodes         map[NodeID]MyTime
	mockMode             bool
	dumpRPC              bool
}

type HttpClient interface {
	Do(r *http.Request) (*http.Response, error)
}

const defaultTimeout = 3 * time.Second

//Creates a new rest api client
//If something like ("www.node:8666",8545) is passed, an error is thrown
func NewClient(ethHost string, mock bool, dump bool) (c *Client, err error) {
	c = &Client{}
	c.mockMode = mock
	c.dumpRPC = dump
	if mock {
		c.httpClient = NewMockClient()
	} else {
		c.httpClient = http.DefaultClient
		c.httpClient.(*http.Client).Timeout = defaultTimeout
	}

	c.DefaultEthNode = ethHost
	c.DefaultRPCPort = strings.Split(ethHost, ":")[1]
	c.seq = 0
	//TODO handle error
	c.LocalInfo, _ = GetLocalInfo()
	c.NetModel = *NewBlockchainNet()
	c.UnreachableAddresses = map[string]MyTime{}
	return
}

//The name says it all
func (rpcClient *Client) SetTimeout(timeout time.Duration) {
	if !rpcClient.mockMode {
		rpcClient.httpClient.(*http.Client).Timeout = defaultTimeout
	}
}


//This is the exposed internal API - one method, so the things like mutex, etc. are possible
//It is possible to pass simple commands or a CallData pointer, through which any results
// will be returned.
//The effect of the call may also be h the NetModel changes, which are visible externally.
func (rpcClient *Client) RPC(data *CallData) (err error) {
	if data == nil {
		return errors.New("No CallData")
	}
	err = rpcClient.actualRpcCall(data)
	return err
}

//Just a sequence to number the rest calls (the "id" field)
//TODO: wrap the sequence as a in a Type
func (rpcClient *Client) nextID() (id uint) {
	id = rpcClient.seq
	rpcClient.seq++
	return
}

//Generic call to the ethereum api's. Uses structures corresponding to the api json specs
//The response gets enclosed in the CallData argument
func (rpcClient *Client) actualRpcCall(data *CallData) error {
	data.Command.Id = rpcClient.nextID()
	jcom, _ := json.Marshal(data.Command)
	//TODO: allow to define and memorize node-specific ports
	host := data.Context.TargetNode
	if !strings.Contains(host, ":") {
		host = host + ":" + rpcClient.DefaultRPCPort
	}
	host = "http://" + host

	req, err := http.NewRequest("POST", host, bytes.NewReader(jcom))
	if err != nil {
		rpcClient.log(fmt.Sprintf("%s", err))
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", rpcClient.UserAgent)
	req.Header.Set("Content-type", "application/json")
	resp, err := rpcClient.httpClient.Do(req)

	if err != nil {
		log.Println(err)
		//rpcClient.NetModel.UnreachableNodes[GhostNode(host)] = MyTime(time.Now())
		return err
	}
	defer resp.Body.Close()

	//Todo: check Response status is 200!!!
	if resp.StatusCode != 200 {
		err = errors.New(resp.Status)
		return err
	}
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		rpcClient.log(fmt.Sprintf("%s", err))
		return err
	}

	if rpcClient.dumpRPC {
		key, _, _ := net.SplitHostPort(req.URL.Host)
		key = key + "_" + data.Command.Method + ".json"
		log.Println("dumping " + key)
		ioutil.WriteFile(key, respBytes, 0644)
	}

	data.JsonRequest = string(jcom)
	var buf bytes.Buffer
	err = json.Indent(&buf, respBytes, "", " ")
	if err != nil {
		rpcClient.log(fmt.Sprint(err))
	} //irrelevant error not worth returning
	data.JsonResponse = buf.String()
	rpcClient.log("Returned:\n" + data.JsonResponse)
	err = Decode(respBytes, data)
	if err != nil {
		rpcClient.log(fmt.Sprint(err))
	}

	return err
}

//The name says it.
// "method" name is needed for constructing the RPC field
//     - which is complete and only the "ID" integer is meant to be changed
func (rpcClient *Client) NewCallData(method string) *CallData {
	com := EthCommand{"2.0", method, []interface{}{}, 0}
	ctx := rpcClient.LocalInfo // Cloning. This at least is my intention ;-)
	calldata := &CallData{Context: ctx, Command: com, Response: EthResponse{}, RandomStuff: map[string]interface{}{}}
	return calldata
}

//Just a stub of a function gathering host system info
func GetLocalInfo() (CallContext, error) {
	hostname, err := os.Hostname()
	conn, err := net.Dial("udp", "8.8.8.8:80")
	var ipaddress string
	if err != nil {
		ipaddress = ""
		log.Println("No network")
	} else {
		defer conn.Close()
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		ipaddress = localAddr.IP.String()
	}
	return CallContext{ClientHostName: hostname, ClientIp: ipaddress}, err
}

func (rpcClient *Client) log(s string) {
	if rpcClient.DebugMode {
		log.Println(s)
	}
}

//validates and formats an RPC method
//if the string passed corresponds to a valid rpc method (modulo upper/lower case)
// - brings to the correct form and return true
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
	"admin_stopWS", "debug_seedHashsign", "eth_gasPrice",
	"eth_accounts", "eth_blockNumber", "eth_getBalance", "eth_getStorageAt",
	"eth_getTransactionCount", "eth_getBlockTransactionCountByHash", "eth_getBlockTransactionCountByNumber",
	"eth_getUncleCountByBlockHash", "web3_clientVersion", "web3_sha3", "net_version", "net_peerCount",
	"net_listening", "eth_protocolVersion", "eth_syncing", "eth_coinbase", "eth_mining", "eth_hashrate",
	"eth_getUncleCountByBlockNumber", "eth_getCode", "eth_sign", "eth_sendTransaction", "eth_sendRawTransaction",
	"eth_call", "eth_estimateGas", "eth_getBlockByHash", "eth_getBlockByNumber", "eth_getTransactionByHash",
	"eth_getTransactionByBlockHashAndIndex", "eth_getTransactionByBlockNumberAndIndex", "var otherCommands",
	"eth_getTransactionReceipt", "eth_getUncleByBlockHashAndIndex", "eth_getUncleByBlockNumberAndIndex", "eth_getCompilers",
	"eth_compileLLL", "eth_compileSolidity" + "eth_compileSerpent", "eth_newFilter", "eth_newBlockFilter", "eth_newPendingTransactionFilter",
	"eth_uninstallFilter", "eth_getFilterChanges", "eth_getFilterLogs", "eth_getLogs", "eth_getWork", "eth_submitWork", "eth_submitHashrate",
	"db_putString", "db_getString", "db_putHex", "db_getHex", "shh_post", "shh_version", "shh_newIdentity", "shh_hasIdentity", "shh_newGroup",
	"shh_addToGroup", "shh_newFilter", "shh_uninstallFilter", "shh_getFilterChanges", "shh_getMessages"}

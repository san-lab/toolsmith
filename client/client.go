package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/san-lab/toolsmith/templates"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
	"io/ioutil"
)

//A rest api client, wrapping an http client
//The struct also contains a map of addresses of known nodes' end-points
//The field Port - to memorize the default Port (a bit of a stretch)
type Client struct {
	DefaultEthNode string
	UserAgent      string
	httpClient     *http.Client
	seq            uint
	r              *templates.Renderer
	LocalInfo      CallContext
	NetModel       BlockchainNet
	DefaultRPCPort  string
	DebugMode      bool
}

const defaultTimeout = 3 * time.Second

//Creates a new rest api client
//If something like ("www.node:8666",8545) is passed, an error is thrown
func NewClient(ethHost string) (c *Client, err error) {
	c = &Client{httpClient: http.DefaultClient}
	c.DefaultEthNode = ethHost
	c.DefaultRPCPort = strings.Split(ethHost, ":")[1]
	c.seq = 0
	c.httpClient.Timeout = defaultTimeout
	//TODO handle error
	c.LocalInfo, _ = GetLocalInfo()
	c.NetModel = *NewBlockchainNet()
	return
}

//The name says it all
func (rpcClient *Client) SetTimeout(timeout time.Duration) {
	rpcClient.httpClient.Timeout = timeout
}

func (rpcClient *Client) DiscoverNetwork() error {
	return rpcClient.scanNetwork(true)
}

func (rpcClient *Client) Rescan() error {
	return rpcClient.scanNetwork(false)
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
	rpcClient.log("About to call: \n" + string(jcom))
	//TODO: allow to define and memorize node-specific ports
	host := data.Context.TargetNode
	if !strings.Contains(host, ":") {
		host = host + ":" + rpcClient.DefaultRPCPort
	}
	host = "http://" + host

	req, err := http.NewRequest("POST", host, bytes.NewReader(jcom))
	if err != nil {
		rpcClient.log(fmt.Sprintf("%s",err))
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", rpcClient.UserAgent)
	req.Header.Set("Content-type", "application/json")
	resp, err := rpcClient.httpClient.Do(req)

	if err != nil {
		fmt.Println(err)
		rpcClient.NetModel.Unreachables[host] = MyTime(time.Now())
		return err
	}
	defer resp.Body.Close()
	//Todo: check Response status is 200!!!
	if resp.StatusCode!=200 {
		err = errors.New(resp.Status)
		return err
	}
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		rpcClient.log(fmt.Sprintf("%s",err))
		return err
	}
	data.JsonRequest = string(jcom)
	var buf bytes.Buffer
	err = json.Indent(&buf, respBytes, "", " ")
	if err != nil {rpcClient.log(fmt.Sprint(err))} //irrelevant error not worth returning
	data.JsonResponse = buf.String()
	rpcClient.log("Returned:\n"+data.JsonResponse)
	err = Decode(respBytes, data)
	if err != nil {rpcClient.log(fmt.Sprint(err))}
	return err
}

//Scan the network following successive peer lists.
//If rebiuld == true discard the old network model
func (rpcClient *Client) scanNetwork(rebuild bool) error {
	if rebuild {
		rpcClient.NetModel.ReachableNodes = map[string]*Node{}
	}
	err := rpcClient.collectNodeInfoRecursively(rpcClient.DefaultEthNode)
	if err != nil {
		return err
	}

	return nil
}

func (rpcClient *Client) HeartBeat() (ok bool, nodes int) {
	if len(rpcClient.NetModel.ReachableNodes) == 0 {
		ok = false
		nodes = 0
		return
	}
	old := int64(rpcClient.NetModel.AccessNode.LastBlockNumberSample.BlockNumber)
	m, err := rpcClient.Bloop()
	if err != nil {
		return false, 0
	}
	var prev int64
	nodes = len(m)
	ok = nodes > 0
	for _, v := range m {
		bns , ok1 := v.(BlockNumberSample)
		if ok = ok1; ! ok {
			return
		}
		bn := int64(bns.BlockNumber)
		if prev == 0 {
			prev = bn
			continue
		}
		if r := bn - prev; r > 2 || r < -2 {
			ok = false
			break
		}

	}
	if prev-old < 1 { //TODO: Elaborate, take timestamp into account
		ok = false
	}
	return
}

func (rpcClient *Client) Bloop() (blocks map[string]interface{}, err error) {
	blocks = map[string]interface{}{}
	for _, node := range rpcClient.NetModel.ReachableNodes {
		data := rpcClient.NewCallData("eth_blockNumber")
		//TODO: preferred address
		var err error
		for target := range node.KnownAddresses {
			data.Context.TargetNode = target
			err = rpcClient.actualRpcCall(data)
			if err == nil {
				break
			}
		}
		if err != nil {
			blocks[node.ShortName()] = "UNREACHABLE!!!"
			fmt.Println(err)
			continue
			//return nil, err
		}
		var ok bool
		node.LastBlockNumberSample, ok = data.ParsedResult.(*BlockNumberSample)
		if !ok {
			fmt.Println("Type assertion failed")
		} else {
			blocks[node.ShortName()] = *node.LastBlockNumberSample
		}

	}
	return
}

//Collect a single node Info, including Peers
//This method will not update the NetworkModek
func (rpcClient *Client) collectNodeInfo(address string) (node *Node, err error) {
	if !strings.Contains(address, ":") {
		address = address + ":" + rpcClient.DefaultRPCPort
	}
	callData := rpcClient.NewCallData("admin_nodeInfo")
	callData.Context.TargetNode = address
	err = rpcClient.actualRpcCall(callData)
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
	node.Status = Active
	node.KnownAddresses[address] = true
	//Get the peer info
	callData = rpcClient.NewCallData("admin_peers")
	callData.Context.TargetNode = address
	err = rpcClient.actualRpcCall(callData)
	if err != nil {
		return node, err //Partially successful method call
	}
	node.Peers, ok = callData.ParsedResult.(*PeerArray)
	if !ok {
		return node, errors.New("Could not parse the result of Peers of " + address)
	}
	//Get the txpool status
	callData.Command.Method = "txpool_status"
	callData.ParsedResult = nil
	err = rpcClient.actualRpcCall(callData)
	if err != nil || !callData.Parsed {
		return node, err
	}
	//Get the BlockNumber
	node.TxpoolStatus = callData.ParsedResult.(*TxpoolStatusSample)
	callData.Command.Method = "eth_blockNumber"
	err = rpcClient.actualRpcCall(callData)
	if err != nil {
		return node, err
	}
	node.LastBlockNumberSample, ok = callData.ParsedResult.(*BlockNumberSample)
	if !ok {
		fmt.Println("Type assertion failed")
	}

	return node, nil
}

//Collect  nodes Info recursing through peers
//The effects are in the NetworkModel
func (rpcClient *Client) collectNodeInfoRecursively(address string) error {
	//First find out the network ID, if not known
	if rpcClient.NetModel.NetworkID == "" {
		callData := rpcClient.NewCallData("net_version")
		callData.Context.TargetNode = address
		err := rpcClient.actualRpcCall(callData)
		if err != nil {
			return err
		}
		sr := callData.ParsedResult.(*StringResult)
		rpcClient.NetModel.NetworkID = string(*sr)
	}

	newnode, err := rpcClient.collectNodeInfo(address)
	if err != nil {
		return err
	}

	if &rpcClient.NetModel.AccessNode == nil || rpcClient.NetModel.AccessNode.Name == "" {
		rpcClient.NetModel.AccessNode = *newnode
	}
	if knownnode, ok := rpcClient.NetModel.ReachableNodes[newnode.ID]; ok {
		//TODO collect addresses
		knownnode.KnownAddresses[address] = true
		fmt.Printf("Found aganin %s\n", knownnode.ID)
	} else { //a new guy!
		rpcClient.NetModel.ReachableNodes[newnode.ID] = newnode
		for _, peer := range *newnode.Peers {
			err := rpcClient.collectNodeInfoRecursively(peer.RemoteHostMachine())
			if err != nil {
				//rpcClient.NetModel.Unreachables[peer.RemoteHostMachine()] = MyTime(time.Now())
				//fmt.Println(err)
			}

		}
	}
	return nil
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

func (rpcClient *Client) log(s string) {
	if rpcClient.DebugMode {
		fmt.Println(s)
	}
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
	"net_listening", "eth_protocolVersion", "eth_syncing", "eth_coinbase", "eth_mining", "eth_hashrate"}
var otherCommands = "eth_getUncleCountByBlockNumber,eth_getCode,eth_sign,eth_sendTransaction,eth_sendRawTransaction,eth_call,eth_estimateGas," +
	"eth_getBlockByHash,eth_getBlockByNumber,eth_getTransactionByHash,eth_getTransactionByBlockHashAndIndex," +
	"eth_getTransactionByBlockNumberAndIndex," +
	"eth_getTransactionReceipt,eth_getUncleByBlockHashAndIndex,eth_getUncleByBlockNumberAndIndex,eth_getCompilers,eth_compileLLL," +
	"eth_compileSolidity,eth_compileSerpent,eth_newFilter,eth_newBlockFilter,eth_newPendingTransactionFilter,eth_uninstallFilter," +
	"eth_getFilterChanges,eth_getFilterLogs,eth_getLogs,eth_getWork,eth_submitWork,eth_submitHashrate,db_putString,db_getString," +
	"db_putHex,db_getHex,shh_post,shh_version,shh_newIdentity,shh_hasIdentity,shh_newGroup,shh_addToGroup,shh_newFilter," +
	"shh_uninstallFilter,shh_getFilterChanges,shh_getMessages"

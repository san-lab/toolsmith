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
	httpClient           *http.Client
	seq                  uint
	r                    *templates.Renderer
	LocalInfo            CallContext
	NetModel             BlockchainNet
	DefaultRPCPort       string
	DebugMode            bool
	UnreachableAddresses map[string]MyTime
	VisitedNodes	map[NodeID]MyTime
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
	c.UnreachableAddresses = map[string]MyTime{}
	return
}

//The name says it all
func (rpcClient *Client) SetTimeout(timeout time.Duration) {
	rpcClient.httpClient.Timeout = timeout
}

func (rpcClient *Client) DiscoverNetwork() error {
	return nil
}

func (rpcClient *Client) Rescan() error {
	rpcClient.UnreachableAddresses = map[string]MyTime{}
	rpcClient.SetNetworkId()
	data := rpcClient.NewCallData("admin_nodeInfo")
	data.Context.TargetNode = rpcClient.DefaultEthNode
	err := rpcClient.actualRpcCall(data)
	if err != nil {
		return err
	}
	ni, ok := data.ParsedResult.(*NodeInfo)
	if !ok {
		log.Printf("expected %T got %T", ni, data.ParsedResult)
		return errors.New("Not ok parsing the root node info")
	}
	rootnode := NodeFromNodeInfo(ni)
	rootnode.KnownAddresses[rpcClient.DefaultEthNode] = true
	rpcClient.NetModel.Nodes[rootnode.ID] = rootnode
	rpcClient.VisitedNodes =  map[NodeID]MyTime{}  //Boy this is ugly!
	rpcClient.collectNodeInfoRecursively(rootnode)
	return nil
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

func (rpcClient *Client) HeartBeat() (ok bool, nodes int) {
	if len(rpcClient.NetModel.Nodes) == 0 {
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
		bns, ok1 := v.(BlockNumberSample)
		if ok = ok1; !ok {
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
	for _, node := range rpcClient.NetModel.Nodes {
		if !node.Reachable {
			continue
		}
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

func (rpcClient *Client) SetNetworkId() error {
	//First find out the network ID, if not known
	if rpcClient.NetModel.NetworkID == "" {
		callData := rpcClient.NewCallData("net_version")
		callData.Context.TargetNode = rpcClient.DefaultEthNode
		err := rpcClient.actualRpcCall(callData)
		if err != nil {
			return err
		}
		sr := callData.ParsedResult.(*StringResult)
		rpcClient.NetModel.NetworkID = string(*sr)
	}
	return nil
}

//Update a  node Info, includig peers, txpool and block number
func (rpcClient *Client) collectNodeInfo(node *Node) (err error) {
	log.Println("Collecting node info on " + node.ShortName())
	if len(node.ID) == 0 {
		return errors.New("Cannot get info of a blank node")
	}
	rpcClient.NetModel.Nodes[node.ID] = node
	callData := rpcClient.NewCallData("admin_peers")
	var prefaddr string
	err = errors.New(fmt.Sprintf("No known/working address for %s", node.ShortName()))
	for address := range node.KnownAddresses { //Dial on all numbers
		if _, ok := rpcClient.UnreachableAddresses[address]; ok {
			continue
		}
		callData.Context.TargetNode = address
		err = rpcClient.actualRpcCall(callData)
		if err == nil {
			prefaddr = address
			break
		}
		rpcClient.UnreachableAddresses[address] = MyTime(time.Now())

	}
	if err != nil { //no contact on any address
		node.Reachable = false
		return err
	}
	node.Reachable = true
	var ok bool
	node.JSONPeers, ok = callData.ParsedResult.(*PeerArray)
	if !ok {
		return errors.New("Could not parse the result of JSONPeers of " + prefaddr)
	}

	for _, pi := range *node.JSONPeers {
		pn := rpcClient.NetModel.Nodes[NodeID(pi.ID)]
		if pn == nil {
			pn = NodeFromPeerInfo(&pi)
			rpcClient.NetModel.Nodes[NodeID(pi.ID)] = pn
		}
		pn.KnownAddresses[pi.RemoteHostMachine()] = true
		log.Printf("Adding %s as a peer of %s\n", pn.ShortName(), node.ShortName())
		node.Peers[pi.RemoteHostMachine()] = *pn
	}
	//Get the txpool status
	callData.Command.Method = "txpool_status"
	callData.ParsedResult = nil
	err = rpcClient.actualRpcCall(callData)
	if err != nil || !callData.Parsed {
		return err
	}
	//Get the BlockNumber
	node.TxpoolStatus = callData.ParsedResult.(*TxpoolStatusSample)
	callData.Command.Method = "eth_blockNumber"
	err = rpcClient.actualRpcCall(callData)
	if err != nil {
		return err
	}
	node.LastBlockNumberSample, ok = callData.ParsedResult.(*BlockNumberSample)
	if !ok {
		log.Println("Type assertion failed")
	}
	return nil
}

//Collect  nodes Info recursing through peers
//The effects are in the NetworkModel
func (rpcClient *Client) collectNodeInfoRecursively(parent *Node) error {
	err := rpcClient.collectNodeInfo(parent)
	if err != nil {
		return err
	}
	rpcClient.VisitedNodes[parent.ID] = MyTime(time.Now())

	for _, peernode := range parent.Peers {
		if _, beenThere :=rpcClient.VisitedNodes[peernode.ID]; !beenThere {
			rpcClient.collectNodeInfoRecursively(&peernode) //ignoring the connection error - the unreachables set already
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
		log.Println(s)
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
	"net_listening", "eth_protocolVersion", "eth_syncing", "eth_coinbase", "eth_mining", "eth_hashrate",
	"eth_getUncleCountByBlockNumber", "eth_getCode", "eth_sign", "eth_sendTransaction", "eth_sendRawTransaction",
	"eth_call", "eth_estimateGas", "eth_getBlockByHash", "eth_getBlockByNumber", "eth_getTransactionByHash",
	"eth_getTransactionByBlockHashAndIndex", "eth_getTransactionByBlockNumberAndIndex", "var otherCommands",
	"eth_getTransactionReceipt", "eth_getUncleByBlockHashAndIndex", "eth_getUncleByBlockNumberAndIndex", "eth_getCompilers",
	"eth_compileLLL", "eth_compileSolidity" + "eth_compileSerpent", "eth_newFilter", "eth_newBlockFilter", "eth_newPendingTransactionFilter",
	"eth_uninstallFilter", "eth_getFilterChanges", "eth_getFilterLogs", "eth_getLogs", "eth_getWork", "eth_submitWork", "eth_submitHashrate",
	"db_putString", "db_getString", "db_putHex", "db_getHex", "shh_post", "shh_version", "shh_newIdentity", "shh_hasIdentity", "shh_newGroup",
	"shh_addToGroup", "shh_newFilter", "shh_uninstallFilter", "shh_getFilterChanges", "shh_getMessages"}

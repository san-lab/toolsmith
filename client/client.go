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
	DefaultEthNodeAddr   string
	UserAgent            string
	httpClient           HttpClient
	seq                  uint
	renderer             *templates.Renderer
	LocalInfo            CallContext
	NetModel             BlockchainNet
	DefaultRPCPort       string
	DebugMode            bool
	UnreachableAddresses map[string]MyTime
	MockMode             bool
	dumpRPC              bool
	blockedAddresses     map[string]bool
}

type HttpClient interface {
	Do(r *http.Request) (*http.Response, error)
}

const defaultTimeout = 3 * time.Second

//Creates a new rest api client
//If something like ("www.node:8666",8545) is passed, an error is thrown
func NewClient(ethHost string, mock bool, dump bool) (c *Client, err error) {
	c = &Client{}
	c.MockMode = mock
	c.dumpRPC = dump
	if mock {
		c.httpClient = NewMockClient()
	} else {
		c.httpClient = http.DefaultClient
		c.httpClient.(*http.Client).Timeout = defaultTimeout
	}

	c.DefaultEthNodeAddr = ethHost
	c.DefaultRPCPort = strings.Split(ethHost, ":")[1]
	c.seq = 0
	//TODO handle error
	c.LocalInfo, _ = GetLocalInfo()
	c.NetModel = *NewBlockchainNet()
	c.UnreachableAddresses = map[string]MyTime{}
	c.blockedAddresses = map[string]bool{}
	return
}

//The name says it all
func (rpcClient *Client) SetTimeout(timeout time.Duration) {
	if !rpcClient.MockMode {
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
	if rpcClient.blockedAddresses[data.Context.TargetNode] {
		return errors.New("Blocked address:" + data.Context.TargetNode)
	}
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
	rpcClient.log("Returned:\n" + fmt.Sprintf("%s", resp.Header))
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
func CamelCaseKnownCommand(command *string) bool { //TODO differentiate between Parity and Geth
	for _, set := range GethCommsSet {
		for _, cc := range set {
			if strings.EqualFold(*command, cc) {
				*command = cc
				return true
			}
		}
	}
	for _, set := range ParityCommsSet {
		for _, cc := range set {
			if strings.EqualFold(*command, cc) {
				*command = cc
				return true
			}
		}
	}
	return false
}

//Add all possible peers
func (rpcClient *Client) FullMesh() error {
	for k1, n1 := range rpcClient.NetModel.Nodes {
		for k2, n2 := range rpcClient.NetModel.Nodes {
			if k1 == k2 {
				continue
			}
			_, hasalready := n1.PeerSeenAs(n2)
			if hasalready {
				continue
			}
			for addr := range n2.KnownAddresses {
				enode := "enode://" + string(n2.ID) + "@" + addr + ":30304"
				callData := rpcClient.NewCallData("admin_addPeer")
				callData.Context.TargetNode = n1.PrefAddress()
				callData.Command.Params = []interface{}{enode}
				err := rpcClient.actualRpcCall(callData)
				if err != nil {
					log.Println(err)
				} else {
					break
				}
			}

		}
	}
	return nil
}

//Artificially block calls to certain address
func (rpcClient *Client) BlockAddress(addr string) {
	rpcClient.blockedAddresses[addr] = true
}

//Remove artificial block on an address
func (rpcClient *Client) UnblockAddress(addr string) {
	delete(rpcClient.blockedAddresses, addr)
}

var GethCommsSet = [][]string{GethRpcMinerComms, GethRpcTxpoolComms, GethRpcAdminComms, GethRpcOtherComms, GenericRpcEthComms, GenericRpcWeb3Comms, GenericRpcNetComms}

var ParityCommsSet = [][]string{GenericRpcWeb3Comms, GenericRpcNetComms, GenericRpcEthComms, RpcPersonalComms, RpcParityComms, RpcParityAccountsComms,
	RpcParitySetComms, RpcParityPubsubComms, RpcParitySignerComms, RpcParityTraceComms, RpcParityShhComms, RpcParitySecretstoreComms}

var GethRpcOtherComms = []string{"debug_backtraceAt", "personal_ecRecover",
	"debug_blockProfile", "miner_setGasPrice", "personal_importRawKey", "txpool_inspect",
	"debug_cpuProfile", "miner_start", "personal_listAccounts", "txpool_status", "debug_dumpBlock", "miner_stop", "personal_lockAccount",
	"debug_gcStats", "miner_getHashrate", "personal_newAccount", "debug_getBlockRlp", "miner_aetEtherbase", "personal_unlockAccount",
	"debug_goTrace", "personal_sendTransaction", "debug_memStats", "personal_sign", "debug_seedHashsign",
	"db_putString", "db_getString", "db_putHex", "db_getHex", "shh_post", "shh_version", "shh_newIdentity", "shh_hasIdentity", "shh_newGroup",
	"shh_addToGroup", "shh_newFilter", "shh_uninstallFilter", "shh_getFilterChanges", "shh_getMessages"}

var GethRpcMinerComms = []string{"miner_setExtra", "miner_setGasPrice", "miner_start", "miner_stop", "miner_getHashrate", "miner_getEtherbase"}

var GethRpcTxpoolComms = []string{"txpool_content", "txpool_inspect", "txpool_status"}

var GethRpcAdminComms = []string{"admin_addPeer", "admin_datadir", "admin_nodeInfo", "admin_peers", "admin_setSolc",
	"admin_startRPC", "admin_startWS", "admin_stopRPC", "admin_stopWS"}

var GenericRpcEthComms = []string{"eth_gasPrice", "eth_accounts", "eth_blockNumber", "eth_getBalance", "eth_getStorageAt",
	"eth_getTransactionCount", "eth_getBlockTransactionCountByHash", "eth_getBlockTransactionCountByNumber", "eth_getUncleCountByBlockHash", "eth_protocolVersion",
	"eth_syncing", "eth_coinbase", "eth_mining", "eth_hashrate",
	"eth_getUncleCountByBlockNumber", "eth_getCode", "eth_sign", "eth_sendTransaction", "eth_sendRawTransaction",
	"eth_call", "eth_estimateGas", "eth_getBlockByHash", "eth_getBlockByNumber", "eth_getTransactionByHash",
	"eth_getTransactionByBlockHashAndIndex", "eth_getTransactionByBlockNumberAndIndex",
	"eth_getTransactionReceipt", "eth_getUncleByBlockHashAndIndex", "eth_getUncleByBlockNumberAndIndex", "eth_getCompilers",
	"eth_compileLLL", "eth_compileSolidity", "eth_compileSerpent", "eth_newFilter", "eth_newBlockFilter", "eth_newPendingTransactionFilter",
	"eth_uninstallFilter", "eth_getFilterChanges", "eth_getFilterLogs", "eth_getLogs", "eth_getWork", "eth_submitWork", "eth_submitHashrate"}

var GenericRpcWeb3Comms = []string{"web3_clientVersion", "web3_sha3"}

var GenericRpcNetComms = []string{"net_listening", "net_peerCount", "net_version"}

var RpcPersonalComms = []string{"personal_listAccounts", "personal_newAccount", "personal_sendTransaction",
	"personal_signTransaction", "personal_unlockAccount", "personal_sign", "personal_ecRecover"}

var RpcParityComms = []string{"parity_cidV0", "parity_composeTransaction", "parity_consensusCapability", "parity_decryptMessage",
	"parity_encryptMessage", "parity_futureTransactions", "parity_allTransactions", "parity_getBlockHeaderByNumber", "parity_listOpenedVaults",
	"parity_listStorageKeys", "parity_listVaults", "parity_localTransactions", "parity_releasesInfo", "parity_signMessage", "parity_versionInfo",
	"parity_changeVault", "parity_changeVaultPassword", "parity_closeVault", "parity_getVaultMeta", "parity_newVault", "parity_openVault",
	"parity_setVaultMeta", "parity_accountsInfo", "parity_checkRequest", "parity_defaultAccount", "parity_generateSecretPhrase",
	"parity_hardwareAccountsInfo", "parity_listAccounts", "parity_phraseToAddress", "parity_postSign", "parity_postTransaction",
	"parity_defaultExtraData", "parity_extraData", "parity_gasCeilTarget", "parity_gasFloorTarget", "parity_minGasPrice", "parity_transactionsLimit",
	"parity_devLogs", "parity_devLogsLevels", "parity_chain", "parity_chainId", "parity_chainStatus", "parity_gasPriceHistogram",
	"parity_netChain", "parity_netPeers", "parity_netPort", "parity_nextNonce", "parity_pendingTransactions", "parity_pendingTransactionsStats",
	"parity_registryAddress", "parity_removeTransaction", "parity_rpcSettings", "parity_unsignedTransactionsCount",
	"parity_dappsUrl", "parity_enode", "parity_mode", "parity_nodeKind", "parity_nodeName", "parity_wsUrl"}
var RpcParityAccountsComms = []string{"parity_allAccountsInfo", "parity_changePassword", "parity_deriveAddressHash", "parity_deriveAddressIndex",
	"parity_exportAccount", "parity_getDappAddresses", "parity_getDappDefaultAddress", "parity_getNewDappsAddresses", "parity_getNewDappsDefaultAddress",
	"parity_importGethAccounts", "parity_killAccount", "parity_listGethAccounts", "parity_listRecentDapps", "parity_newAccountFromPhrase",
	"parity_newAccountFromSecret", "parity_newAccountFromWallet", "parity_removeAddress", "parity_setAccountMeta", "parity_setAccountName",
	"parity_setDappAddresses", "parity_setDappDefaultAddress", "parity_setNewDappsAddresses", "parity_setNewDappsDefaultAddress",
	"parity_testPassword"}

var RpcParitySetComms = []string{"parity_acceptNonReservedPeers", "parity_addReservedPeer", "parity_dappsList", "parity_dropNonReservedPeers",
	"parity_executeUpgrade", "parity_hashContent", "parity_removeReservedPeer", "parity_setAuthor", "parity_setChain", "parity_setEngineSigner",
	"parity_setExtraData", "parity_setGasCeilTarget", "parity_setGasFloorTarget", "parity_setMaxTransactionGas", "parity_setMinGasPrice",
	"parity_setMode", "parity_setTransactionsLimit", "parity_upgradeReady"}
var RpcParityPubsubComms = []string{"parity_subscribe", "parity_unsubscribe"}

var RpcParitySignerComms = []string{"signer_confirmRequest", "signer_confirmRequestRaw", "signer_confirmRequestWithToken", "signer_generateAuthorizationToken",
						"signer_generateWebProxyAccessToken", "signer_rejectRequest", "signer_requestsToConfirm", "signer_subscribePending", "signer_unsubscribePending"}
var RpcParityTraceComms = []string{}       //TODO
var RpcParityShhComms = []string{}         //TODO
var RpcParitySecretstoreComms = []string{} //TODO

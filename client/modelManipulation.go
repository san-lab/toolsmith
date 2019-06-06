package client

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

func (rpcClient *Client) Rescan() error {
	for _, node := range rpcClient.NetModel.Nodes {
		err := rpcClient.collectNodeInfo(node, true)
		if err != nil {
			log.Println(err)
		}
	}
	return nil
}

func (rpcClient *Client) collectNodeInfo(node *Node, refetch bool) error {

	err := rpcClient.establishNodeClientVersion(node, refetch)
	if err != nil {
		return err
	}

	if node.IsGeth() {
		return rpcClient.collectGethNodeInfo(node, refetch)
	} else if node.IsParity() {
		return rpcClient.collectParityNodeInfo(node)
	} else {
		return errors.New("unsupported Eth client")
	}
}

func (rpcClient *Client) establishNodeClientVersion(stub *Node, refetch bool) (err error) {
	if refetch || len(stub.ClientVersion) == 0 {
		addr := stub.PrefAddress()
		if len(addr) == 0 {
			err = errors.New("no known address for the node, exiting")
			log.Println(err)
			return err
		}

		data := rpcClient.NewCallData("web3_clientVersion")
		data.Context.TargetNode = addr
		err = rpcClient.actualRpcCall(data)
		if err != nil {
			return err
		}
		cvr, ok := data.ParsedResult.(*StringResult)
		if !ok {
			err = errors.New("could not parse the ClientVersion of the root node")
			return err
		}
		stub.SetReachable(true)
		stub.ClientVersion = string(*cvr)

	}
	return nil

}

func (rpcClient *Client) SetPeers(node *Node) (err error) {
	var method string

	if node.IsGeth() {
		method = "admin_peers"
	} else if node.IsParity() {
		method = "parity_netPeers"
	} else {
		err = errors.New("unsupported Client version: " + node.ClientVersion)
		return
	}
	data := rpcClient.NewCallData(method)
	data.Context.TargetNode = node.prefAddress
	err = rpcClient.actualRpcCall(data)
	if err != nil || !data.Parsed {
		return
	}
	if node.IsGeth() {
		var ok bool
		node.JSONPeers, ok = data.ParsedResult.(*PeerArray)
		if !ok {
			err = errors.New("Could not parse the result of JSONPeers of " + node.ShortName)
			return
		}
	} else if node.IsParity() {
		peersResp, ok := data.ParsedResult.(*ParityPeerInfo)
		if !ok {
			errors.New("Could not parse the result of JSONPeers of " + node.ShortName)
			return
		}
		node.JSONPeers = &peersResp.Peers

	}

	for _, pi := range *node.JSONPeers {
		n := NodeFromPeerInfo_Get(nil, &pi)
		node.Peers[NodeID(pi.ID)] = n
	}

	return
}

func (rpcClient *Client) collectGethNodeInfo(node *Node, fromScratch bool) error {
	data := rpcClient.NewCallData("admin_nodeInfo")
	data.Context.TargetNode = node.PrefAddress()
	if fromScratch {

		err := rpcClient.actualRpcCall(data)
		if err != nil {
			return err
		}
		ni, ok := data.ParsedResult.(*NodeInfo)
		if !ok {
			log.Printf("expected %T got %T", ni, data.ParsedResult)
			return errors.New("not ok parsing the root node info")
		}
		FillNodeFromNodeInfo_Geth(node, ni)
		node.isFromPeer = false
	}
	//Get the txpool status
	data.Command.Method = "txpool_status"
	data.ParsedResult = nil
	err := rpcClient.actualRpcCall(data)
	if err != nil || !data.Parsed {
		return err
	}
	node.TxpoolStatus = data.ParsedResult.(*TxpoolStatusSample)

	//Get the BlockNumber
	err = node.sampleBlockNo(rpcClient)

	//Get peers
	err = rpcClient.SetPeers(node)

	return err
}

func (rpcClient *Client) collectParityNodeInfo(stub *Node) error {
	data := rpcClient.NewCallData("parity_nodeName")
	data.Context.TargetNode = stub.PrefAddress()
	err := rpcClient.actualRpcCall(data)
	if err != nil {
		return err
	}
	stub.ShortName = string(*data.ParsedResult.(*StringResult))
	data.Command.Method = "parity_enode"
	err = rpcClient.actualRpcCall(data)
	if err != nil {
		return err
	}
	stub.Enode = string(*data.ParsedResult.(*StringResult)) //TODO: parsing errors unhandled!
	stub.ID = NodeID(strings.Split(strings.Split(stub.Enode, "//")[1], "@")[0])
	stub.FullName = stub.ShortName + "/" + stub.Enode
	stub.isFromPeer = false
	//TODO: this is fitting Parity info into Geth structures - ugly
	data.Command.Method = "parity_pendingTransactions"
	err = rpcClient.actualRpcCall(data)
	if err != nil {
		return err
	}
	err = stub.sampleBlockNo(rpcClient)
	if err != nil {
		return err
	}
	txs, ok := data.ParsedResult.(*ParityPendingTxs)
	if !ok {
		return errors.New("could not cast to parity_pendingTransactions while getting node info")
	}
	stub.TxpoolStatus = &TxpoolStatusSample{Pending: HexString(txs.Len()), Queued: HexString(0)}
	stub.TxpoolStatus.stamp()

	//Get peers
	err = rpcClient.SetPeers(stub)
	return err
}

func (rpcClient *Client) DiscoverNetwork() error {
	rpcClient.UnreachableAddresses = map[string]MyTime{}
	rpcClient.NetModel.Nodes = map[NodeID]*Node{}
	err := rpcClient.GetNetworkBasics()
	if err != nil {
		return err
	}
	rpcClient.collectNodeInfoRecursively(rpcClient.NetModel.Nodes[rpcClient.NetModel.AccessNodeID])
	return nil
}

//Collect  nodes Info recursing through peers
//The effects are in the NetworkModel
func (rpcClient *Client) collectNodeInfoRecursively(parent *Node) error {

	for id, peer := range parent.Peers {
		if rpcClient.NetModel.Nodes[id] == nil {
			node := NewNode()
			node.ID = peer.ID
			node.prefAddress = peer.prefAddress
			node.KnownAddresses[peer.prefAddress] = true
			rpcClient.NetModel.Nodes[node.ID] = node
			rpcClient.collectNodeInfo(node, true)

			rpcClient.collectNodeInfoRecursively(node)
		}
	}

	return nil
}

var Threshold = time.Second * 15
var previousSample map[NodeID]int64
var previousSampleTime time.Time

//Returns block-progress flag and the number of unreachable nodes and non-progressing nodes
//Returns -1 as the number of unreachables if not enough time since previous probe
func (rpcClient *Client) HeartBeat() (progress bool, unreachables int, stucknodes int) {
	rpcClient.Rescan()
	for _, node := range rpcClient.NetModel.Nodes {
		if !node.IsReachable() {
			unreachables++
			continue
		}
		progress = progress || node.progress
		if !node.progress {
			stucknodes++
			continue
		}
	}
	return
}

func (rpcClient *Client) Bloop() (blocks map[string]interface{}, err error) {
	blocks = map[string]interface{}{}
	for _, node := range rpcClient.NetModel.Nodes {
		err := node.sampleBlockNo(rpcClient)
		if err != nil {
			blocks[node.ShortName] = "UNREACHABLE!!!"
			fmt.Println(err)
			continue
			//return nil, err
		}
		blocks[node.ShortName] = *node.LastBlockNumberSample

	}
	return
}

//Verifies that the default RPC gateway works, Gets the basic Network information
//Gets basic info on the entry Node and Peers of the entry Node
func (rpcClient *Client) GetNetworkBasics() error {
	//First find out the network ID, if not known

	callData := rpcClient.NewCallData("net_version")
	callData.Context.TargetNode = rpcClient.DefaultEthNodeAddr
	err := rpcClient.actualRpcCall(callData)
	if err != nil {
		return err
	}
	sr := callData.ParsedResult.(*StringResult)
	rpcClient.NetModel.NetworkID = string(*sr)
	stub := NewNode()
	stub.KnownAddresses[rpcClient.DefaultEthNodeAddr] = true
	stub.prefAddress = rpcClient.DefaultEthNodeAddr
	err = rpcClient.collectNodeInfo(stub, true)
	if err != nil {
		return err
	}
	rpcClient.NetModel.AccessNodeID = stub.ID
	rpcClient.NetModel.Nodes[stub.ID] = stub

	return nil
}

func (rpcClient *Client) collectNodePeerInfo(node *Node, rebuild bool) (err error) {

	//_, err = rpcClient.Peers(node.PrefAddress())
	if err != nil {
		return
	}
	node.Peers = map[NodeID]*Node{}
	for _, pi := range *node.JSONPeers {

		pn := NodeFromPeerInfo_Get(nil, &pi)

		pn.KnownAddresses[pi.RemoteHostMachine()] = true

	}
	return nil
}

func (n *Node) sampleBlockNo(rpc *Client) error {
	callData := rpc.NewCallData("eth_blockNumber")
	callData.Context.TargetNode = n.PrefAddress()
	err := rpc.actualRpcCall(callData)
	if err != nil || !callData.Parsed {
		return err
	}
	blockNumberSample, ok := callData.ParsedResult.(*BlockNumberSample)
	if !ok {
		err = errors.New("type assertion failed")
		log.Println(err)
		return err
	}
	if n.LastBlockNumberSample == nil {
		n.LastBlockNumberSample = blockNumberSample
		return nil
	}
	if blockNumberSample.BlockNumber > n.LastBlockNumberSample.BlockNumber {
		n.PrevBlockNumberSample = n.LastBlockNumberSample
		n.LastBlockNumberSample = blockNumberSample
		n.progress = true
	} else {
		if time.Time(blockNumberSample.Sampled).Sub(time.Time(n.LastBlockNumberSample.Sampled)) > Threshold {
			n.progress = false
		}
	}
	return nil
}

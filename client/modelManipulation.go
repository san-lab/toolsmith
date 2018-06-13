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

func (rpcClient *Client) collectNodeInfo(node *Node, flag bool) error {
	if node.isStub {
		err := rpcClient.collectStubNodeInfo(node)
		if err != nil {
			return err
		}
	}
	if node.IsGeth() {
		return rpcClient.collectGethNodeInfo(node, flag)
	} else if node.IsParity() {
		return rpcClient.collectParityNodeInfo(node)
	} else {
		return errors.New("unsupported Eth client")
	}
}

func (rpcClient *Client) collectStubNodeInfo(stub *Node) (err error) {
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
	return nil
}

//This is a swiss-army-knife method, and hence the addr as an argument instead of a node pointer
func (rpcClient *Client) Peers(addr string) (pinfo PeerArray, err error) {
	var method string
	node, found := rpcClient.NetModel.ResolveAddress(addr)
	if !found {
		node = NewNode()
		node.KnownAddresses[rpcClient.DefaultEthNodeAddr] = true
		rpcClient.NetModel.AccessNode = node
		err = rpcClient.collectNodeInfo(node, true)
		if err != nil {
			return
		}
		rpcClient.NetModel.Nodes[node.ID] = node
	}

	if node.IsGeth() {
		method = "admin_peers"
	} else if node.IsParity() {
		method = "parity_netPeers"
	} else {
		err = errors.New("unsupported Client version: " + node.ClientVersion)
		return
	}
	data := rpcClient.NewCallData(method)
	data.Context.TargetNode = addr
	err = rpcClient.actualRpcCall(data)
	if err != nil || !data.Parsed {
		return
	}
	if node.IsGeth() {
		var ok bool
		node.JSONPeers, ok = data.ParsedResult.(*PeerArray)
		pinfo = *node.JSONPeers
		if !ok {
			err = errors.New("Could not parse the result of JSONPeers of " + node.ShortName + " at " + addr)
			return
		}
	} else if node.IsParity() {
		ppeersResp, ok := data.ParsedResult.(*ParityPeerInfo)
		if !ok {
			errors.New("Could not parse the result of JSONPeers of " + node.ShortName + " at " + addr)
			return
		}
		node.JSONPeers = &ppeersResp.Peers
		pinfo = ppeersResp.Peers

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
		FillNodeFromNodeInfo(node, ni)
		node.isStub = false
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
	stub.Name = stub.ShortName + "/" + stub.Enode
	stub.isStub = false
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
	return err
}

func (rpcClient *Client) DiscoverNetwork() error {
	rpcClient.UnreachableAddresses = map[string]MyTime{}
	rpcClient.NetModel.Nodes = map[NodeID]*Node{}
	rpcClient.SetNetworkId()
	stub := NewNode()
	stub.KnownAddresses[rpcClient.DefaultEthNodeAddr] = true
	rpcClient.NetModel.AccessNode = stub
	err := rpcClient.collectNodeInfo(stub, true)
	if err != nil {
		return err
	}
	rpcClient.NetModel.Nodes[stub.ID] = stub
	rpcClient.collectNodeInfoRecursively(stub)
	return nil
}

//Collect  nodes Info recursing through peers
//The effects are in the NetworkModel
func (rpcClient *Client) collectNodeInfoRecursively(parent *Node) error {

	err := rpcClient.collectNodePeerInfo(parent, true)
	if err != nil {
		return err
	}

	for _, peernode := range parent.Peers {
		if _, visited := rpcClient.NetModel.Nodes[peernode.ID]; visited {
			continue
		}
		rpcClient.NetModel.Nodes[peernode.ID] = peernode
		err = rpcClient.collectNodeInfo(peernode, false)
		if err != nil {
			log.Println(err)
			continue
		}

		err = rpcClient.collectNodeInfoRecursively(peernode)
		if err != nil {
			log.Println(err)
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

func (rpcClient *Client) SetNetworkId() error {
	//First find out the network ID, if not known
	if rpcClient.NetModel.NetworkID == "" {
		callData := rpcClient.NewCallData("net_version")
		callData.Context.TargetNode = rpcClient.DefaultEthNodeAddr
		err := rpcClient.actualRpcCall(callData)
		if err != nil {
			return err
		}
		sr := callData.ParsedResult.(*StringResult)
		rpcClient.NetModel.NetworkID = string(*sr)
	}
	return nil
}

func (rpcClient *Client) collectNodePeerInfo(node *Node, rebuild bool) (err error) {
	/*
		var method string

		if node.IsGeth() {
			method = "admin_peers"
		} else if node.IsParity() {
			method = "parity_netPeers"
		} else {
			return errors.New("unsupported Client version: " + node.ClientVersion)
		}
		prefaddr := node.PrefAddress()
		if len(prefaddr) == 0 {
			err = errors.New("cannot find peers, no known address for the node " + node.ShortName)
			return
		}
		callData := rpcClient.NewCallData(method)
		callData.Context.TargetNode = prefaddr
		err = rpcClient.actualRpcCall(callData)
		if err != nil || !callData.Parsed {
			return err
		}

		if node.IsGeth() {
			var ok bool
			node.JSONPeers, ok = callData.ParsedResult.(*PeerArray)
			if !ok {
				return errors.New("Could not parse the result of JSONPeers of " + node.ShortName + " at " + prefaddr)
			}
		} else if node.IsParity() {
			ppeersResp, ok := callData.ParsedResult.(*ParityPeerInfo)
			if !ok {
				return errors.New("Could not parse the result of JSONPeers of " + node.ShortName + " at " + prefaddr)
			}
			node.JSONPeers = &ppeersResp.Peers
		}
	*/
	_, err = rpcClient.Peers(node.PrefAddress())
	if err != nil {
		return
	}
	node.Peers = map[string]*Node{}
	for _, pi := range *node.JSONPeers {
		var pn *Node
		var known bool
		if pn, known = rpcClient.NetModel.Nodes[NodeID(pi.ID)]; known && !rebuild {
			NodeFromPeerInfo(pn, &pi)
		} else {
			pn = NodeFromPeerInfo(nil, &pi)
		}
		pn.KnownAddresses[pi.RemoteHostMachine()] = true
		node.Peers[pi.RemoteHostMachine()] = pn
		//log.Printf("Adding %s as a peer of %s\n", pn.ShortName(), node.ShortName())

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

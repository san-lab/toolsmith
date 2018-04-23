package client

import (
	"errors"
	"fmt"
	"log"
	"time"
)



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




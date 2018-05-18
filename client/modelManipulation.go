package client

import (
	"errors"
	"fmt"
	"log"
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

func (rpcClient *Client) DiscoverNetwork() error {
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
	//rpcClient.NetModel.Nodes = map[NodeID]*Node{}
	rootnode := NodeFromNodeInfo(ni)
	rootnode.KnownAddresses[rpcClient.DefaultEthNode] = true
	rpcClient.NetModel.Nodes[rootnode.ID] = rootnode
	rpcClient.NetModel.AccessNode = rootnode
	rpcClient.VisitedNodes = map[NodeID]MyTime{} //Boy this is ugly!
	rpcClient.collectNodeInfoRecursively(rootnode)
	return nil
}


var threshold = time.Second*15
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
			blocks[node.ShortName()] = "UNREACHABLE!!!"
			fmt.Println(err)
			continue
			//return nil, err
		}
		blocks[node.ShortName()] = *node.LastBlockNumberSample

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

//Update a  node Info, includig peers, txpool and block number. If "insist", the method will retry calling on previously unreachable addresses
func (rpcClient *Client) collectNodeInfo(node *Node, insist bool) (err error) {
	//log.Println("Collecting node info on " + node.ShortName() +"/" + node.IDHead(6))
	if len(node.ID) == 0 {
		return errors.New("cannot get info of a blank node")
	}
	rpcClient.NetModel.FindOrAddNode(node)
	callData := rpcClient.NewCallData("admin_peers")
	var prefaddr string
	err = errors.New(fmt.Sprintf("No known/working address for %s", node.ShortName()))
	for address := range node.KnownAddresses { //Dial on all numbers
		if _, ok := rpcClient.UnreachableAddresses[address]; ok {
			log.Println("Hit an unrachable address: " + address)
			if insist {
				delete(rpcClient.UnreachableAddresses, address)
			} else {
				continue
			}

		}
		callData.Context.TargetNode = address
		err = rpcClient.actualRpcCall(callData)
		if err == nil {
			prefaddr = address
			node.SetReachable(true)
			break
		}
		rpcClient.UnreachableAddresses[address] = MyTime(time.Now())

	}
	if err != nil { //no contact on any address
		node.SetReachable ( false)
		return err
	}
	node.SetReachable(true)
	node.prefAddress=prefaddr
	var ok bool
	node.JSONPeers, ok = callData.ParsedResult.(*PeerArray)
	if !ok {
		return errors.New("Could not parse the result of JSONPeers of " + prefaddr)
	}
	node.Peers = map[string]*Node{}
	for _, pi := range *node.JSONPeers {
		pn, exists := rpcClient.NetModel.Nodes[NodeID(pi.ID)]
		if !exists {
			pn = NodeFromPeerInfo(&pi)
			rpcClient.NetModel.FindOrAddNode(pn)

		}
		pn.KnownAddresses[pi.RemoteHostMachine()] = true
		//log.Printf("Adding %s as a peer of %s\n", pn.ShortName(), node.ShortName())
		node.Peers[pi.RemoteHostMachine()] = pn
	}
	//Get the txpool status
	callData.Command.Method = "txpool_status"
	callData.ParsedResult = nil
	err = rpcClient.actualRpcCall(callData)
	if err != nil || !callData.Parsed {
		return err
	}
	node.TxpoolStatus = callData.ParsedResult.(*TxpoolStatusSample)

	//Get the BlockNumber
	err = node.sampleBlockNo(rpcClient)

	return err
}

func (n *Node) sampleBlockNo(rpc *Client) error {
	callData := rpc.NewCallData("eth_blockNumber")
	callData.Context.TargetNode=n.PrefAddress()
	err := rpc.actualRpcCall(callData)
	if err != nil || ! callData.Parsed {
		return err
	}
	blockNumberSample, ok := callData.ParsedResult.(*BlockNumberSample)
	if !ok {
		err = errors.New("Type assertion failed")
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
		if time.Time(blockNumberSample.Sampled).Sub(time.Time(n.LastBlockNumberSample.Sampled)) > threshold {
			n.progress=false
		}
	}
	return nil
}



//Collect  nodes Info recursing through peers
//The effects are in the NetworkModel
func (rpcClient *Client) collectNodeInfoRecursively(parent *Node) error {
	err := rpcClient.collectNodeInfo(parent, false)
	if err != nil {
		return err
	}
	rpcClient.VisitedNodes[parent.ID] = MyTime(time.Now())

	for _, peernode := range parent.Peers {
		if _, beenThere := rpcClient.VisitedNodes[peernode.ID]; !beenThere {
			rpcClient.collectNodeInfoRecursively(peernode) //ignoring the connection error - the unreachables set already
		}
	}
	return nil
}

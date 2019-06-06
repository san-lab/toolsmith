package client

import (
	"log"
	"strings"
	"time"
)

//A structure to keep the model of the blockchain network
type BlockchainNet struct {
	DefaultAccessIP   string
	DefaultEthRpcPort string
	AccessNodeID      NodeID
	NetworkID         string
	Nodes             map[NodeID]*Node //The NodeID is meant to be the key here

}

func NewBlockchainNet() *BlockchainNet {
	bl := &BlockchainNet{}
	bl.Nodes = map[NodeID]*Node{}
	return bl
}

type MyTime time.Time

func (mt MyTime) String() string {
	return time.Time(mt).Format(time.RFC1123)
}

type NodeStatus string

const Unknown NodeStatus = "unknown"
const Active NodeStatus = "active"
const Syncing NodeStatus = "syncing"
const Stalled NodeStatus = "stalled"
const Unreachable NodeStatus = "unreachable"

type NodeID string

//A structure to keep the information about a single node of the BC network
type Node struct {
	Status                NodeStatus
	ID                    NodeID
	Enode                 string
	ThisNodeInfo          NodeInfo // should not be needed
	FullName              string
	ShortName             string
	KnownAddresses        map[string]bool
	JSONPeers             *PeerArray // Should not be needed
	LastBlockNumberSample *BlockNumberSample
	PrevBlockNumberSample *BlockNumberSample
	TxpoolStatus          *TxpoolStatusSample
	Peers                 map[NodeID]*Node //
	LastReach             MyTime
	LastFail              MyTime
	issReachable          bool
	prefAddress           string
	progress              bool
	ClientVersion         string
	isFromPeer            bool
}

func (n *Node) IsStuck() bool {
	return !n.progress
}

func (bcn BlockchainNet) ResolveAddress(addr string) (*Node, bool) {
	for _, n := range bcn.Nodes {
		if n.KnownAddresses[addr] {
			return n, true
		}
	}
	return nil, false
}

//Returns address at which node1 see node2 as peer  and false if no connection detected
func (n *Node) PeerSeenAs(peer NodeID) (string, bool) {
	for _, p := range n.Peers {
		if p.ID == peer {
			return p.prefAddress, true
		}
	}
	return "", false
}

// Geth specific
// Extract the short name from the NodeAddress name
// assuming the name is of the form: "Geth/miner3/v1.7.2-stable/linux-amd64/go1.9.2"
func (n Node) getGethShortName() (string, bool) {
	if !n.IsGeth() {
		return "", false
	}
	parts := strings.Split(n.FullName, "/")
	if len(parts) > 1 {
		return parts[1], true

	}
	return "unknown", false
}

//This is a stub. The address does not include the rpc port
//The bool flag is true if an actual address is returned, false otherwise
func (n Node) PrefAddress() string {
	if len(n.prefAddress) > 0 {
		return n.prefAddress
	}
	for a, ok := range n.KnownAddresses {
		if ok {
			n.prefAddress = a
			return a
		}
	}
	return ""
}

func (n Node) IDHead(i int) string {
	if len(n.ID) < i {
		return ""
	}
	return string(n.ID)[:i]
}

func (n Node) IDTail(i int) string {
	if len(n.ID) < i {
		return ""
	}
	return string(n.ID)[len(n.ID)-i:]
}

func (n *Node) SetReachable(is bool) {
	log.Printf("Setting %s as reachable=%v\n", n.ShortName, is)
	n.issReachable = is
	if is {
		n.LastReach = MyTime(time.Now())
	} else {
		n.LastFail = MyTime(time.Now())
	}
}

func (n *Node) IsReachable() bool {
	return n.issReachable
}

func (n *Node) IsParity() bool {
	return strings.HasPrefix(n.ClientVersion, "Parity")
}

func (n *Node) IsGeth() bool {
	return strings.HasPrefix(n.ClientVersion, "Geth")
}

func (n *Node) IsPantheon() bool {
	return strings.HasPrefix(n.ClientVersion, "pantheon")
}

func (n *Node) ClientType() string {
	return strings.Split(n.ClientVersion, "/")[0]
}

func NewNode() *Node {
	n := &Node{}
	n.KnownAddresses = map[string]bool{}
	n.Peers = map[NodeID]*Node{}
	n.isFromPeer = true
	return n
}

//Geth specific
func FillNodeFromNodeInfo_Geth(n *Node, ni *NodeInfo) {
	n.ID = NodeID(ni.ID)
	n.ThisNodeInfo = *ni
	n.FullName = ni.Name
	n.Enode = ni.Enode
	n.ShortName, _ = n.getGethShortName()
	n.SetReachable(true) //This is based on the assumption that the node info has been just obtained
	return
}

func NodeFromPeerInfo_Get(n *Node, pi *PeerInfo) *Node {
	if n == nil {
		n = NewNode()
	}
	n.ID = NodeID(pi.ID)
	n.FullName = pi.Name
	n.ShortName, _ = n.getGethShortName()
	addr := strings.Split(pi.Network.RemoteAddress, ":")[0]
	n.KnownAddresses = map[string]bool{addr: true}
	n.prefAddress = addr
	return n
}

func (bcn *BlockchainNet) isOk() bool {
	for k, v := range bcn.Nodes {
		if k != v.ID {
			log.Fatal("false key %s for node %s\n", k, v.ID)
			return false
		}
	}
	log.Println("Model OK")
	return true
}

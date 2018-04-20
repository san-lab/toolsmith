package client

import (
	"strings"
	"time"
)

//A structure to keep the model of the blockchain network
type BlockchainNet struct {
	DefaultAccessIP   string
	DefaultEthrpcPort string
	AccessNode        Node
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
	ThisNodeInfo          NodeInfo // should not be needed
	Name                  string
	KnownAddresses        map[string]bool
	JSONPeers             *PeerArray // Shouuld not be needed
	LastBlockNumberSample *BlockNumberSample
	TxpoolStatus          *TxpoolStatusSample
	Peers                 map[string]Node // ipaddress -> node
	LastPing              MyTime
	Reachable             bool
}

func (nm BlockchainNet) ResolveAddress(addr string) (*Node, bool) {
	for _, n := range nm.Nodes {
		if n.KnownAddresses[addr] {
			return n, true
		}
	}
	return nil, false
}

//Extract the short name from the NodeAddress name
// assuming the name is of the form: "Geth/miner3/v1.7.2-stable/linux-amd64/go1.9.2"
func (n *Node) ShortName() string {
	parts := strings.Split(n.Name, "/")
	if len(parts) > 1 {
		return parts[1]
	}
	return n.Name
}

//This is a stub. The address does not include the rpc port
//TODO: actual smarts for selecting the preferred address
func (n *Node) PrefAddress() string {
	for a, ok := range n.KnownAddresses {
		if ok {
			return a
		}
	}
	return ""
}

func (n *Node) IDHead(i int) string {
	return string(n.ID)[:i]
}

func (n *Node) IDTail(i int) string {
	return string(n.ID)[len(n.ID)-i:]
}

func NewNode() *Node {
	n := &Node{}
	n.KnownAddresses = map[string]bool{}
	n.Peers = map[string]Node{}
	return n
}

func NodeFromNodeInfo(ni *NodeInfo) *Node {
	n := NewNode()
	n.ID = NodeID(ni.ID)
	n.ThisNodeInfo = *ni
	n.Name = ni.Name
	n.Reachable = true //This is based on the assumption that the node info has been just obtained
	n.LastPing = MyTime(time.Now())
	return n
}

func NodeFromPeerInfo(pi *PeerInfo) *Node {
	n := NewNode()
	n.ID = NodeID(pi.ID)
	n.Name = pi.Name
	n.KnownAddresses = map[string]bool{}
	return n
}

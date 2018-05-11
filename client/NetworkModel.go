package client

import (
	"log"
	"strings"
	"time"
)

//A structure to keep the model of the blockchain network
type BlockchainNet struct {
	DefaultAccessIP   string
	DefaultEthrpcPort string
	AccessNode        *Node
	NetworkID         string
	Nodes             map[NodeID]*Node //The NodeID is meant to be the key here

}

func (bcn *BlockchainNet) FindOrAddNode(nn *Node) bool {
	//defer bcn.listReaches()
	on, has := bcn.Nodes[nn.ID]
	if has {
		*nn = *on
		return true
	} else {
		bcn.Nodes[nn.ID] = nn
		return false
	}
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
	JSONPeers             *PeerArray // Should not be needed
	LastBlockNumberSample *BlockNumberSample
	PrevBlockNumberSample *BlockNumberSample
	TxpoolStatus          *TxpoolStatusSample
	Peers                 map[string]*Node // ipaddress -> node
	LastReach             MyTime
	LastFail			 MyTime
	issReachable           bool
	prefAddress string
	progress bool
}

func (bcn BlockchainNet) ResolveAddress(addr string) (*Node, bool) {
	for _, n := range bcn.Nodes {
		if n.KnownAddresses[addr] {
			return n, true
		}
	}
	return nil, false
}

func (n *Node) PeerSeenAs(peer *Node) (string, bool) {
	for a, p := range n.Peers {
		if p.ID == peer.ID {
			return a, true
		}
	}
	return "invisible", false
}

//Extract the short name from the NodeAddress name
// assuming the name is of the form: "Geth/miner3/v1.7.2-stable/linux-amd64/go1.9.2"
func (n Node) ShortName() string {
	parts := strings.Split(n.Name, "/")
	if len(parts) > 1 {
		return parts[1]
	}
	return n.Name
}

//This is a stub. The address does not include the rpc port
func (n Node) PrefAddress() string {
	if len(n.prefAddress) > 0 {
		return n.prefAddress
	}
	for a, ok := range n.KnownAddresses {
		if ok {
			return a
		}
	}
	return ""
}

func (n Node) IDHead(i int) string {
	return string(n.ID)[:i]
}

func (n Node) IDTail(i int) string {
	return string(n.ID)[len(n.ID)-i:]
}

func (n *Node) SetReachable(is bool) {
	log.Printf("Setting %s as reachable=%v\n", n.ShortName(), is)
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

func NewNode() *Node {
	n := &Node{}
	n.KnownAddresses = map[string]bool{}
	n.Peers = map[string]*Node{}
	return n
}

func NodeFromNodeInfo(ni *NodeInfo) *Node {
	n := NewNode()
	n.ID = NodeID(ni.ID)
	n.ThisNodeInfo = *ni
	n.Name = ni.Name
	n.SetReachable(true) //This is based on the assumption that the node info has been just obtained
	return n
}

func NodeFromPeerInfo(pi *PeerInfo) *Node {
	n := NewNode()
	n.ID = NodeID(pi.ID)
	n.Name = pi.Name
	n.KnownAddresses = map[string]bool{}
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

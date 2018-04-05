package client

import (
	"time"

	"strings"
)

//A structure to keep the model of the blockchain network
type BlockchainNet struct {
	DefaultAccessIP   string
	DefaultEthrpcPort string
	AccessNode        Node
	NetworkID         string
	ReachableNodes    map[string]*Node  //The NodeID is meant to be the key here
	Unreachables      map[string]MyTime //The address/ip is the key. The last connect failure is the value
}

func NewBlockchainNet() *BlockchainNet {
	bl := &BlockchainNet{}
	bl.ReachableNodes = map[string]*Node{}
	bl.Unreachables = map[string]MyTime{}
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

//A structure to keep the information about a single node of the BC network
type Node struct {
	Status                NodeStatus
	ID                    string
	ThisNodeInfo          NodeInfo
	Name                  string
	KnownAddresses        map[string]bool
	Peers                 *PeerArray
	LastBlockNumberSample *BlockNumberSample
	TxpoolStatus          *TxpoolStatusSample
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
	for a := range n.KnownAddresses {
		return a
	}
	return ""
}

func (n *Node) IDHead(i int) string {
	return n.ID[:i]
}

func (n *Node) IDTail(i int) string {
	return n.ID[len(n.ID)-i:]
}

func NewNode() *Node {
	n := &Node{}
	n.Status = Unknown
	n.ThisNodeInfo = NodeInfo{}
	n.KnownAddresses = map[string]bool{}
	return n
}

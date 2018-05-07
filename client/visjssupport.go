package client

import (
	"encoding/json"
	"html/template"
	"log"
)

func (bcn *BlockchainNet) VisjsNodes() template.JS {
	vn := bcn.GetJsonNodes()
	ret, err := json.Marshal(vn)
	if err != nil {
		log.Println(err)
	}
	return template.JS(ret)
}

func (bcn *BlockchainNet) GetJsonNodes() []Visnode {
	vn := []Visnode{}
	for _, nd := range bcn.Nodes {
		vi := Visnode{Id: nd.ID, Label: nd.ShortName(), Image: "/static/ethereum_32x32.png", Shape: "image"}
		if nd.Reachable {vi.Image="/static/ethereum-full_32x32.png"}
		for a := range nd.KnownAddresses {
			vi.Label = vi.Label + "\n" + a
		}
		vn = append(vn, vi)
	}
	return vn
}

func (bcn *BlockchainNet) VisjsEdges() template.JS {
	ve := []Visedge{}
	for _, nd := range bcn.Nodes {
		for adr, pnd := range nd.Peers {
			if nd.ID > pnd.ID {
				retAddr, _ := pnd.PeerSeenAs(nd)
				ve = append(ve, VisjsEdge(nd, adr+"<->"+retAddr, pnd))
			}
		}

	}

	ret, err := json.Marshal(ve)
	if err != nil {
		log.Println(err, ret)
	}
	return template.JS(ret)
}

func VisjsEdge(base *Node, addr string, peer *Node) Visedge {
	e := Visedge{From: base.ID, To: peer.ID, Label: addr}
	e.Color.Color = "blue"
	e.Color.Highlight = "blue"
	e.Color.Hover = "green"
	e.Smooth = false
	f := Font{Size: 1, Align: "bottom"}
	e.Font = f
	return e
}

type Visnode struct {
	Id    NodeID `json:"id,omitempty"`
	Label string `json:"label"`
	Image string `json:"image"`
	Shape string `json:"shape"`
	Color Color  `json:"color,omitempty"`
}

type Visedge struct {
	ID     string `json:"id,omitempty"`
	From   NodeID `json:"from"`
	To     NodeID `json:"to"`
	Arrows string `json:"arrows,omitempty"`
	Label  string `json:"label"`
	Font   Font   `json:"font,omitempty"`
	Color  Color  `json:"color,omitempty"`
	Smooth bool   `json:"smooth"`
}

type Color struct {
	Color     string `json:"color"`
	Highlight string `json:"highlight,omitempty"`
	Hover     string `json:"hover,omitempty"`
}

type Font struct {
	Size  int    `json:"size,omitempty"`
	Align string `json:"align,omitempty"`
}

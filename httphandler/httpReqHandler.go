package httphandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"watchcat/client"
	"watchcat/templates"
)

const toggle = "togglerawmode"
const discover = "discovernetwork"

var KnownLocalCommands = []string{toggle, discover}

//This is the glue between the http requests and the (hopefully) generic RPC client

type LilHttpHandler struct {
	//defaultContext client.CallContext
	rpcClient *client.Client
	r         *templates.Renderer
}

//TODO: Config variable
func NewHttpHandler(c Config) (lhh *LilHttpHandler, err error) {
	lhh = &LilHttpHandler{}
	lhh.r = templates.NewRenderer()
	lhh.rpcClient, err = client.NewClient(c.EthHost, c.EthPort)
	return lhh, err
}

// Handles incoming requests. Some will be forwarted to the RPC client.
// Assumes the request path has either: 1 part - interpreted as a /command with logic implemented within the client
//                                  or: 2 parts - interpreted as /node/ethMethod
// The port No set at Client initialization is used for the RPC call
func (lhh *LilHttpHandler) Handler(w http.ResponseWriter, r *http.Request) {
	if r.FormValue(toggle) == "yes" {
		lhh.rpcClient.LocalInfo.RawMode = !lhh.rpcClient.LocalInfo.RawMode
	}
	isSlash := func(c rune) bool { return c == '/' }
	f := strings.FieldsFunc(r.URL.Path, isSlash)
	fmt.Println(f)
	switch len(f) {
	case 1:
		comm := f[0]
		lhh.GenericCommand(w, r, comm)
	case 2:
		eNode := f[0]
		eMethod := f[1]
		lhh.RpcCallAndRespond(w, r, eNode, eMethod)
	default:
		lhh.r.RenderResponse(w, "home", lhh.rpcClient)
	}
}

//TODO
func (lhh *LilHttpHandler) GenericCommand(w http.ResponseWriter, r *http.Request, comm string) {
	switch comm {
	case discover:
		lhh.rpcClient.FirstCall()
		lhh.r.RenderResponse(w, "network", lhh.rpcClient.NetModel)
		//fmt.Fprintf(w,"%s\n" , "Not implemented")
	case toggle:
		//lhh.rpcClient.LocalInfo.RawMode = !c.LocalInfo.RawMode
	default:
		lhh.RpcCallAndRespond(w, r, lhh.rpcClient.DefaultEthNode, comm)
	}

}

// Ethereum RPC call to the eNode and rendering the appropriate HTML result page.
// Parameter values from the url query will be marshaled as json params[],
// but if there are multiple values for any particular key, only the first value will be used.
// The parameter names will be skipped.
func (lhh *LilHttpHandler) RpcCallAndRespond(w http.ResponseWriter, r *http.Request, eNode string, eMethod string) {
	client.CamelCaseKnownCommand(&eMethod)
	var err error
	r.ParseForm()
	callData := lhh.rpcClient.NewCallData(eMethod)
	callData.Context.TargetNode = eNode
	callData.Context.RequestPath = r.RequestURI

	//TODO: assure parameter ordering
	for _, v := range r.Form {
		callData.Command.Params = append(callData.Command.Params, v[0])
	}
	var showRaw bool
	//if len(r.Form["showRaw"])>0  && r.Form["showRaw"][0]=="true" {
	//	showRaw=true
	//}
	if r.FormValue("showRaw") == "true" || callData.Context.RawMode {
		showRaw = true
	}
	err = lhh.rpcClient.ActualRpcCall(callData)
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	//TODO refactor the showRaw mode
	// Spurious ---------------------
	var jcom, jres []byte
	jcom, err = json.Marshal(callData.Command)
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	jres, err = json.MarshalIndent(callData.Response, "", "   ")
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	callData.RandomStuff["Jcom"] = string(jcom)
	callData.RandomStuff["Jres"] = string(jres)
	// End of Spurious --------------

	if showRaw {
		lhh.r.RenderResponse(w, "raw", callData)
	} else {
		switch eMethod {
		case "eth_blockNumber":
			lhh.r.RenderResponse(w, "blockNumber", callData)
		case "admin_peers":
			lhh.r.RenderResponse(w, "peers", callData)
		default:
			lhh.r.RenderResponse(w, "raw", callData)

		}
	}

}

type Config struct {
	EthPort  string
	EthHost  string
	HttpPort string
}

package httphandler

import (
	"errors"
	"fmt"
	"github.com/san-lab/toolsmith/client"
	"github.com/san-lab/toolsmith/templates"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

const toggle = "togglerawmode"
const discover = "discovernetwork"
const bloop = "bloop"
const rescan = "rescan"
const heartbeat = "heartbeat"

var KnownLocalCommands = []string{toggle, discover}

//This is the glue between the http requests and the (hopefully) generic RPC client

type LilHttpHandler struct {
	//defaultContext client.CallContext
	config    Config
	rpcClient *client.Client
	r         *templates.Renderer
}

//Creating a naw http handler with its embedded rpc client and html renderer
func NewHttpHandler(c Config) (lhh *LilHttpHandler, err error) {
	lhh = &LilHttpHandler{}
	lhh.config = c
	lhh.r = templates.NewRenderer()
	lhh.rpcClient, err = client.NewClient(c.EthHost)
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
		if client.CamelCaseKnownCommand(&comm) {
			lhh.RpcCallAndRespond(w, r, lhh.config.EthHost, comm)
		} else {
			lhh.GenericCommand(w, r, comm)
		}
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
	var err error
	switch comm {
	case discover:
		err = lhh.rpcClient.DiscoverNetwork()
		lhh.r.RenderResponse(w, "network", lhh.rpcClient.NetModel)
	case rescan:
		err = lhh.rpcClient.Rescan()
		lhh.r.RenderResponse(w, "network", lhh.rpcClient.NetModel)
	case bloop:
		m, _ := lhh.rpcClient.Bloop()
		lhh.r.RenderResponse(w, "listMap", m)
	case heartbeat:
		ok, nodes := lhh.rpcClient.HeartBeat()
		fmt.Fprintf(w, "Network heartbeat: %v for %v nodes", ok, nodes)
	default:
		err = errors.New(fmt.Sprintf("Unknown command: %s", comm))
	}
	if err != nil {
		fmt.Fprintln(w, err)
	}
}

// Ethereum RPC call to the eNode and rendering the appropriate HTML result page.
// Parameter values from the url query will be marshaled as json params[], if their keys are of the form "parX", where X=0..9
// but if there are multiple values for any particular key, only the first value will be used.
// The parameter names will be skipped.
func (lhh *LilHttpHandler) RpcCallAndRespond(w http.ResponseWriter, r *http.Request, eNode string, eMethod string) {
	client.CamelCaseKnownCommand(&eMethod) //We could stop here if false, but what if there are new methods?
	var err error
	r.ParseForm()
	callData := lhh.rpcClient.NewCallData(eMethod)
	callData.Context.TargetNode = eNode
	callData.Context.RequestPath = r.RequestURI

	//"parN"=parameterValue
	paramValidator := regexp.MustCompile(`par\d$`)
	var keys []string
	for k := range r.Form {
		if paramValidator.MatchString(k) {
			keys = append(keys, k)

		}
	}
	if len(keys) > 0 {
		sort.Strings(keys)
		for _, pk := range keys {
			callData.Command.Params = append(callData.Command.Params, r.Form[pk][0])
		}
	}
	// End of param handling

	var showRaw bool
	//showRaw parameter is independent of the current value of the RawMode
	if r.FormValue("showRaw") == "true" || callData.Context.RawMode {
		showRaw = true          //for rendering
		callData.RawJson = true //for decoding
	}
	err = lhh.rpcClient.RPC(callData)
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}

	if showRaw {
		lhh.r.RenderResponse(w, "raw", callData)
	} else {
		switch eMethod {
		case "eth_blockNumber":
			lhh.r.RenderResponse(w, "blockNumber", callData)
		case "admin_peers":
			lhh.r.RenderResponse(w, "peers", callData)
		case "txpool_status":
			lhh.r.RenderResponse(w, "txpoolStatus", callData)
		default:
			lhh.r.RenderResponse(w, "raw", callData)

		}
	}

}

type Config struct {
	EthHost  string
	HttpPort string
}

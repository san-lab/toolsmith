package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/san-lab/toolsmith/client"
	"github.com/san-lab/toolsmith/templates"
	"github.com/san-lab/toolsmith/watchdog"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const toggle = "togglerawmode"
const discover = "discovernetwork"
const bloop = "bloop"
const rescan = "rescan"
const heartbeat = "heartbeat"
const peers = "peers"
const debugOn = "debugon"
const debugOff = "debugoff"
const loadtemplates = "loadtemplates"
const magic = "magicone"
const nodesJSON = "jsonnodes"
const mockblock = "mockblock"
const mockunblock = "mockunblock"
const rawnodes = "rawnodes"
const fullmesh = "fullmesh"
const addrecipient = "addrecipient"
const blockrecipient = "blockrecipient"
const removerecipient = "removerecipient"
const emailparamname = "addr" //param name
const setwatchdoginterval = "setwatchdoginterval"
const watchdogstatus = "watchdogstatus"
const setwatchdogstatusok = "setwatchdogstatusok"
const interval = "interval" //param name
const setpassword = "setpassword"
const setthreshold = "setthreshold"
const threshold = "threshold" // param name

const passwdFile = "http.passwd.json"

//This is the glue between the http requests and the (hopefully) generic RPC client

type LilHttpHandler struct {
	//defaultContext client.CallContext
	config    Config
	rpcClient *client.Client
	renderer  *templates.Renderer
	watchdog  *watchdog.Watchdog
}

//Creating a naw http handler with its embedded rpc client and html renderer
func NewHttpHandler(c Config, ctx context.Context) (lhh *LilHttpHandler, err error) {
	lhh = &LilHttpHandler{}
	lhh.config = c
	lhh.renderer = templates.NewRenderer()
	lhh.rpcClient, err = client.NewClient(c.EthHost, c.MockMode, c.DumpRPC)
	if c.StartWatchdog {
		lhh.watchdog = watchdog.StartWatchdog(lhh.rpcClient, ctx)
	}
	if c.BasicAuth {
		lhh.initPasswords(ctx)
	}
	return lhh, err
}

// Handles incoming requests. Some will be forwarded to the RPC client.
// Assumes the request path has either: 1 part - interpreted as a /command with logic implemented within the client
//                                  or: 2 parts - interpreted as /node/ethMethod
// The port No set at Client initialization is used for the RPC call
func (lhh *LilHttpHandler) Handler(w http.ResponseWriter, r *http.Request) {
	// FormValue() does the call to Parse()
	if r.FormValue(toggle) == "yes" {
		lhh.rpcClient.LocalInfo.RawMode = !lhh.rpcClient.LocalInfo.RawMode
	}
	isSlash := func(c rune) bool { return c == '/' }
	f := strings.FieldsFunc(r.URL.Path, isSlash)
	//log.Println(f)
	switch len(f) {
	case 1:
		comm := f[0]
		if client.CamelCaseKnownCommand(&comm) {
			lhh.RpcCallAndRespond(w, r, lhh.config.EthHost, comm)
		} else if strings.HasPrefix(comm, "json") {
			lhh.handleJSON(w, r, comm)
		} else {
			lhh.SpecialCommand(w, r, comm)
		}
	case 2:
		eNode := f[0]
		eMethod := f[1]
		lhh.RpcCallAndRespond(w, r, eNode, eMethod)
	default:
		cc := lhh.rpcClient.LocalInfo
		if lhh.rpcClient.NetModel.NetworkID == "" {
			lhh.rpcClient.DiscoverNetwork()
		}
		rdata := templates.RenderData{TemplateName: "magic", HeaderData: &cc, Client: lhh.rpcClient}
		err := lhh.renderer.RenderResponse(w, rdata)
		if err != nil {
			log.Println(err)
		}
	}
}

type handler func(w http.ResponseWriter, r *http.Request)

func (lhh *LilHttpHandler) GetHandler(withAuth bool) handler {
	if withAuth {
		return lhh.BasicAuthHandler
	}
	return lhh.Handler
}

func (lhh *LilHttpHandler) SpecialCommand(w http.ResponseWriter, r *http.Request, comm string) {
	var err error
	cc := lhh.rpcClient.LocalInfo
	cc.Watchdog = lhh.watchdog != nil
	if cc.Watchdog {
		cc.WatchdogInterval = lhh.watchdog.GetInterval()
	}
	rdata := templates.RenderData{HeaderData: &cc, TemplateName: templates.Home, Client: lhh.rpcClient}
	switch comm {
	case peers:
		node, ok := lhh.rpcClient.NetModel.Nodes[client.NodeID(r.FormValue("nodeid"))]
		if ok {
			rdata.BodyData = node
			rdata.TemplateName = templates.Peers
		}
	case discover:
		err = lhh.rpcClient.DiscoverNetwork()
		rdata.TemplateName = templates.Network
		rdata.BodyData = lhh.rpcClient.NetModel
	case rescan:
		err = lhh.rpcClient.Rescan()
		rdata.TemplateName = templates.Network
		rdata.BodyData = lhh.rpcClient.NetModel
	case bloop:
		m, _ := lhh.rpcClient.Bloop()
		rdata.TemplateName = templates.ListMap
		rdata.BodyData = m
		rdata.HeaderData.SetRefresh(5)
	case heartbeat:
		ok, nodesu, nodess := lhh.rpcClient.HeartBeat()
		fmt.Fprintf(w, "%s>  \n progress: %v, \n unreachable %v, \n stuck %v", client.MyTime(time.Now()), ok, nodesu, nodess)
		return
		//rdata.Error = fmt.Sprintf("Heartbeat: %s for the %v nodes reachable", ok, nodes) //A hack!
	case debugOff:
		lhh.rpcClient.DebugMode = false
	case debugOn:
		lhh.rpcClient.DebugMode = true
	case magic:
		rdata.TemplateName = "magic"
		lhh.rpcClient.Rescan()
		rdata.BodyData = &lhh.rpcClient.NetModel
	case loadtemplates:
		lhh.renderer.LoadTemplates()
	case rawnodes:
		rdata.TemplateName = "nodelist"
		lhh.rpcClient.Rescan()
	case fullmesh:
		lhh.rpcClient.FullMesh()
		lhh.rpcClient.Rescan()
		rdata.TemplateName = "magic"
	case mockblock:
		lhh.rpcClient.BlockAddress(r.FormValue("addr"))
	case mockunblock:
		lhh.rpcClient.UnblockAddress(r.FormValue("addr"))
	case addrecipient:
		email := r.Form.Get(emailparamname)
		lhh.watchdog.AddRecipient(email)
		rdata.TemplateName = "watchdogstatus"
		rdata.BodyData = lhh.watchdog
	case blockrecipient:
		email := r.Form.Get(emailparamname)
		lhh.watchdog.BlockRecipient(email)
		rdata.TemplateName = "watchdogstatus"
		rdata.BodyData = lhh.watchdog
	case removerecipient:
		email := r.Form.Get(emailparamname)
		lhh.watchdog.RemoveRecipient(email)
		rdata.TemplateName = "watchdogstatus"
		rdata.BodyData = lhh.watchdog
	case setwatchdoginterval:
		i, err := strconv.ParseInt(r.Form.Get(interval), 0, 0)
		if err == nil {
			lhh.watchdog.SetInterval(i)
		}
	case setthreshold:
		i, err := strconv.ParseInt(r.Form.Get(threshold), 0, 0)
		if err == nil {
			lhh.watchdog.SetThreshold(i)
		}
	case setwatchdogstatusok:
		lhh.watchdog.SetStatusOk()
		fallthrough
	case watchdogstatus:
		rdata.BodyData = lhh.watchdog
		rdata.TemplateName = "watchdogstatus"
	case setpassword: //TODO: Move to POST only, cast constans as constants
		username, _, ok := r.BasicAuth()
		if ok {
			npassword, passSent := r.Form["pass"]
			if passSent {
				lhh.setPassword(username, npassword[0])
			}
		}

	default:
		err_msg := fmt.Sprintf("Unknown command: %s", comm)

		err = errors.New(err_msg)
	}
	if err != nil {
		log.Println(err)
		rdata.Error = fmt.Sprint(err)
	}
	lhh.renderer.RenderResponse(w, rdata)

}

func (lhh *LilHttpHandler) handleJSON(writer http.ResponseWriter, rq *http.Request, comm string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(200)
	nodes := lhh.rpcClient.NetModel.GetJsonNodes()
	json.NewEncoder(writer).Encode(nodes)

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
	cc := lhh.rpcClient.LocalInfo //Cloning, i hope
	rdata := templates.RenderData{HeaderData: &cc, BodyData: callData}
	if showRaw {
		rdata.TemplateName = templates.Raw
	} else {
		switch eMethod {
		case "eth_blockNumber":
			rdata.TemplateName = templates.BlockNumber
		case "admin_peers":
			rdata.TemplateName = templates.Peers
		case "txpool_status":
			rdata.TemplateName = templates.TxpoolStatus
		case "admin_datadir", "net_version":
			rdata.TemplateName = templates.BlockNumber
		default:
			rdata.TemplateName = templates.Raw

		}

	}
	lhh.renderer.RenderResponse(w, rdata)

}

type Config struct {
	EthHost       string
	MockMode      bool
	DumpRPC       bool
	StartWatchdog bool
	BasicAuth     bool
}

# toolsmith
Toolsmith is a little tool written in Go, useful for monitoring and troubleshooting a moderate-size private Ethereum network
I wrote it because I got exasperated with Cackeshop

The design principles have been:
1) Single technology
2) No dependencies (minimal dependencies). A simple tool should not require half of the Internet to compile
3) Trivial install
4) Modular

Toolsmith is easy to install. Actually no install is needed, just compile/unzip and run.
There is a few flags to pass to the executeble:
```
  -dumpRPC
    	should dump RPC responses to files
  -ethRPCAddress string
    	default RPC access point (default "localhost:8545")
  -httpPort string
    	http port (default "8090")
  -mockMode
    	should mock http RPC client
  -startWatchdog
    	should a watchdog  be started
  -withAuth
    	should Basic Authentication be enabled (default true)
   ```  
   The Toolsmith strars with no knowledge of the network apart from the RPC access point (default: "localhost:8545"), so you may need to execute "Rebuild" in order to initialize. Toolsmith will recursively ask nodes for their peers and build its internal network model.
   
   Toolsmith has modular structure. The core is the RPC client. It knows valid Ethereum/Geth RPC calls and can marshal/unmarshal the corresponding JSON messages. The RPC client maintains internally a model of the (so-far-discovered) network in order to be able to diagnose abnormal situations.
   
 1) HttpHandler
 
   HttpHandler provides an HTML frontend to tinteract with the RPC Client.
   The BasicAuthentication is enabled by default, with the default user/password being "sanlab"/"sanlab28660".
   
   The HttpHandler tries to interpret the request's URI as an \<\<nodeIP\>\>/\<\<rpcCommand\>\> followed by potional parameters ?par1=\<\<value1\>\>&par2=\<\<value2\>\>&... The parameter names have to be of the form `par\d$`. The parameter values (if any) will be included in the RPC call in the order determined by the trailing number of the parameter name.
  
If the URI cannot be interpreted as an RPC call, it will be matched against the HttpHandler-specific commands:
```
const discover = "discovernetwork"
const bloop = "bloop"
const rescan = "rescan"
const heartbeat = "heartbeat"
const debugOn = "debugon"
const debugOff = "debugoff"
const loadtemplates = "loadtemplates"
const magic = "magicone"
const toggle = "togglerawmode"
const mockblock = "mockblock"
const mockunblock = "mockunblock"
const rawnodes = "rawnodes"
const fullmesh = "fullmesh"

const setwatchdoginterval = "setwatchdoginterval"; const interval = "interval" //param name
const watchdogstatus = "watchdogstatus"
const setwatchdogstatusok = "setwatchdogstatusok"
const addrecipient = "addrecipient"; const emailparamname = "addr" //param name
const blockrecipient = "blockrecipient"
const removerecipient = "removerecipient"

const setpassword = "setpassword"
const setthreshold = "setthreshold"; const threshold = "threshold" // param name

const nodesJSON = "jsonnodes"
const mockblock = "mockblock"
const mockunblock = "mockunblock"
```

2) Watchdog
   
3) HTML Renderer and the templates
   
   


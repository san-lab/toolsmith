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
      
      


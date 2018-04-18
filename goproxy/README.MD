This is a simple modification of https://gist.github.com/vmihailenco proxy.go

usage of ./goproxy:\
  -callHost string \
    	The server to call (default "localhost")\
  -callPort string\
    	The port to call (default "9090")\
  -headersOnly\
    	If true no message body dump\
  -listenPort string\
    	The listening port (default "8080")\
  -listeningHost string\
    	The listening address. It should be localhost normally (default "localhost")\

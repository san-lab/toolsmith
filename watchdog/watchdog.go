package watchdog

import (
	"sync"
	"github.com/san-lab/toolsmith/client"
	"io/ioutil"
	"log"
	"encoding/json"
	"time"
	"context"
	"regexp"
	"github.com/san-lab/toolsmith/mailer"
	"sync/atomic"
	"fmt"
)

const configFile =  "watchdog.config.json"
const defaultProbeInterval = time.Second*5

type State int
var okState State = 0
var detected State = 1
var notified State = 2
var reset State = 3



type Watchdog struct {
	config Config
	rpcClient *client.Client
	state State
	execContext context.Context
	ticker *time.Ticker
	exitChan chan interface{}
	wg *sync.WaitGroup
}

type Config struct {
	Recipients map[string]bool
	ProbeInterval time.Duration
}

var started uint32
var mx sync.Mutex
var once sync.Once
var instance *Watchdog


//Initializes and starts the single instance of a watchdog
func StartWatchdog(rpcClient *client.Client, ctx context.Context) *Watchdog {
	if atomic.LoadUint32(&started) == 1 {
		return instance
	}
	mx.Lock()
	defer mx.Unlock()
	instance = &Watchdog{rpcClient: rpcClient }
	instance.config = Config{}
	instance.LoadConfig()
	if instance.config.ProbeInterval==0 {
		instance.config.ProbeInterval = defaultProbeInterval
	}
	instance.execContext = ctx
	instance.ticker = time.NewTicker(defaultProbeInterval)
	instance.wg, _ = ctx.Value("WaitGroup").(*sync.WaitGroup)
	instance.wg.Add(1)
	instance.state=reset
	go instance.run()
	return instance
}


func (w *Watchdog) run () {
	defer w.wg.Done()
	for {
		select {
		case <-w.execContext.Done():
			log.Println("Rolling down...")
			w.SaveConfig()
			return
		case <-w.ticker.C:
			w.probe()
		}
	}
}

//TODO mutex this method
func (w *Watchdog) probe() {
	log.Println("Watching out!")
	good, nnodes := w.rpcClient.HeartBeat()

	if good && nnodes == len(w.rpcClient.NetModel.Nodes) {
		w.state= okState
	} else {
		if w.state== okState {
			w.state=detected
			//TODO: use templates
			log.Println("good: ", good, " nnodes: ", nnodes)
			message := fmt.Sprintf("This is a warning from %s:</br> -Blocks being mined: %v </br> -Nodes not responding: %v", w.rpcClient.LocalInfo.ClientIp, good, len(w.rpcClient.NetModel.Nodes)-nnodes)
			mailer.SendEmail(w.ListRecipients(), "Something wrong with Blockchain Net", message, message)
			w.state=notified
		}
	}
}

func (w *Watchdog) GetStatus() State {
	return w.state
}

//in seconds
func (w *Watchdog) SetInterval(interval int64) {
	w.config.ProbeInterval = time.Duration(interval)*time.Second
	w.ticker = time.NewTicker(w.config.ProbeInterval)
}

//in seconds
func (w *Watchdog) GetInterval() int64 {
	return int64(w.config.ProbeInterval/time.Second)

}

//List active recipients in aws-sdk friendly format
func (w *Watchdog) ListRecipients() []*string {
	list := []*string{}
	for em, active := range w.config.Recipients {
		if active {
			list = append(list, &em)
		}
	}
	return list
}

func (w *Watchdog) GetRecipients() map[string]bool {
	rc := w.config.Recipients // is this defensive copying even necessary?
	return rc
}

//If the email is on the list of recipients, set the active flag to false
//returns if the email has been found on the list
func (w *Watchdog) BlockRecipient(email string) bool {
	var ok bool
	if _, ok = w.config.Recipients[email]; ok  {
		w.config.Recipients[email] = false
	}
	return ok
}

//If the email is on the list of recipients, it is removed from it
//returns if the email has been found on the list
func (w *Watchdog) RemoveRecipient(email string) bool {
	var ok bool
	if _, ok = w.config.Recipients[email]; ok  {
		delete(w.config.Recipients,email)
	}
	return ok
}

//Regexp-validates given email. If valid, adds to the recipients list. Returns validation result
func (w *Watchdog) AddRecipient(email string) bool {
	if w.config.Recipients==nil {
		w.config.Recipients=map[string]bool{}
	}
	re := regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
	if re.MatchString(email) {
		w.config.Recipients[email]=true
		return true
	}

	return false
}



func (w *Watchdog) LoadConfig() error {
	buff, err := ioutil.ReadFile("./"+configFile)
	if err != nil {
		log.Println(err)
		return err
	}
	err = json.Unmarshal(buff, &w.config )
	if err != nil {

	}
	return err
}

//Normally invoked only if context.cancel (aka ^C) stops the execution
func (w *Watchdog) SaveConfig() {
	bytes, err := json.Marshal(w.config)
	if err != nil {
		log.Println(err)
		return
	}
	ioutil.WriteFile("./"+configFile, bytes , 0644)
}






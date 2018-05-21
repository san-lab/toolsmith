package watchdog

import (
	"context"
	"encoding/json"
	"github.com/san-lab/toolsmith/client"
	"github.com/san-lab/toolsmith/mailer"
	"io/ioutil"
	"log"
	"regexp"
	"sync"
	"sync/atomic"
	"time"
)

const configFile = "watchdog.config.json"
const defaultProbeInterval = time.Second * 5

type State struct {
	main     string
	severity string
}

var okState = "OK"
var detected = "DETECTED"
var notified = "NOTIFIED"
var reset = "RESET"

func (s *State) isOK() bool {
	return s.main == okState
}
func (s *State) isNotified() bool {
	return s.main == notified
}
func (s *State) isDetected() bool {
	return s.main == detected
}

type Watchdog struct {
	config       Config
	rpcClient    *client.Client
	state        State
	currentIssue string
	execContext  context.Context
	ticker       *time.Ticker
	exitChan     chan interface{}
	wg           *sync.WaitGroup
}

type Config struct {
	Recipients    map[string]bool
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
	instance = &Watchdog{rpcClient: rpcClient}
	instance.config = Config{}
	instance.LoadConfig()
	if instance.config.ProbeInterval == 0 {
		instance.config.ProbeInterval = defaultProbeInterval
	}
	instance.execContext = ctx
	instance.ticker = time.NewTicker(instance.config.ProbeInterval)
	instance.wg, _ = ctx.Value("WaitGroup").(*sync.WaitGroup)
	instance.wg.Add(1)
	instance.state = State{main: reset}
	go instance.run()
	return instance
}

func (w *Watchdog) run() {
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

type notificationType string

var escalate = notificationType("escalate")
var deescalate = notificationType("deescalate")
var none = notificationType("none")

func (w *Watchdog) shouldNotify(s *State) notificationType {
	if w.state.isNotified() && s.isOK() {
		return deescalate
	}
	if w.state.isOK() && s.isDetected() {
		return escalate
	}
	if s.isDetected() && w.state.severity == "AMBER" && s.severity == "RED" {
		return escalate
	}
	return none
}

func (w *Watchdog) probe() {
	mx.Lock()
	defer mx.Unlock()
	log.Println("Watching out!")
	progress, unreach, stuck := w.rpcClient.HeartBeat()

	//Establish the new state
	s := State{}
	if progress {
		if unreach > 0 || stuck > 0 {
			s.main = detected
			s.severity = "AMBER"
		} else {
			s.main = okState
		}
	} else {
		s.main = detected
		s.severity = "RED"
	}

	notif := w.shouldNotify(&s)
	if notif == deescalate {
		message := mailer.GetMailer().RenderOver(w.currentIssue)
		mailer.GetMailer().SendEmail(w.RecipientsAWSStyle(), "Issue: "+w.currentIssue+">> Blochchain network back to normal", message, "it is over")
		w.currentIssue = ""
		w.state.main = okState
		w.state.severity = ""

	} else {
		if notif == escalate {
			w.state = s
			w.currentIssue = w.generateIssueID()

			wAddress := w.rpcClient.LocalInfo.ClientIp
			unr := []string{}
			stk := []string{}
			for _, n := range w.rpcClient.NetModel.Nodes {
				if !n.IsReachable() {
					unr = append(unr, n.ShortName())
				}
				if n.IsStuck() {
					stk = append(stk, n.ShortName())
				}
			}
			var data = struct {
				IssueID          string
				Severity         string
				WatchdogAddress  string
				UnreachableNodes []string
				StuckNodes       []string
			}{
				w.currentIssue, s.severity, wAddress, unr, stk,
			}
			mailer.GetMailer().LoadTemplate() //Debug line...
			message := mailer.GetMailer().RenderAlert(data)
			mailer.GetMailer().SendEmail(w.RecipientsAWSStyle(), "Something wrong with Blockchain Net", message, "alert!")
			w.state.main = notified
		}
	}

}

func (w *Watchdog) generateIssueID() string {
	return time.Now().Format("020120060304")
}

func (w *Watchdog) SetStatusOk() {
	w.state.main = okState
	w.state.severity = ""
}

func (w *Watchdog) GetStatus() State {
	return w.state
}

//in seconds
func (w *Watchdog) SetInterval(interval int64) {
	w.config.ProbeInterval = time.Duration(interval) * time.Second
	w.ticker = time.NewTicker(w.config.ProbeInterval)
}

//in seconds
func (w *Watchdog) GetInterval() int64 {
	return int64(w.config.ProbeInterval / time.Second)

}

//List active recipients in aws-sdk friendly format
func (w *Watchdog) RecipientsAWSStyle() []*string {
	var list []*string
	for em, active := range w.config.Recipients {
		if active {
			tmp := em
			list = append(list, &tmp)
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
	if _, ok = w.config.Recipients[email]; ok {
		w.config.Recipients[email] = false
	}
	return ok
}

//If the email is on the list of recipients, it is removed from it
//returns if the email has been found on the list
func (w *Watchdog) RemoveRecipient(email string) bool {
	var ok bool
	if _, ok = w.config.Recipients[email]; ok {
		delete(w.config.Recipients, email)
	}
	return ok
}

//Regexp-validates given email. If valid, adds to the recipients list. Returns validation result
func (w *Watchdog) AddRecipient(email string) bool {
	if w.config.Recipients == nil {
		w.config.Recipients = map[string]bool{}
	}
	re := regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
	if re.MatchString(email) {
		w.config.Recipients[email] = true
		return true
	}

	return false
}

func (w *Watchdog) LoadConfig() error {
	buff, err := ioutil.ReadFile("./" + configFile)
	if err != nil {
		log.Println(err)
		return err
	}
	err = json.Unmarshal(buff, &w.config)
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
	ioutil.WriteFile("./"+configFile, bytes, 0644)
}

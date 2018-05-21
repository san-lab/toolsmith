package httphandler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
)

func (lhh *LilHttpHandler) BasicAuthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
	username, password, authOK := r.BasicAuth()
	if authOK && validateAuth(username, password) {
		lhh.Handler(w, r)
		return
	}
	http.Error(w, "Not authorized", 401)
}

func validateAuth(username, password string) bool {
	arrSha := sha256.Sum224([]byte(password))
	orig := hex.EncodeToString(arrSha[:])
	v, has := passMap[username]
	return has && orig == v

}

var passMap map[string]string

//Try to read username/password pairs from a file.
//If it does not work, sets the defult pair of sanlab:sanlab2018
func (lhh *LilHttpHandler) initPasswords(ctx context.Context) {
	passMap = map[string]string{}
	var err, err2 error
	buff, err := ioutil.ReadFile("./" + passwdFile)

	if err != nil {
		log.Println(err)
	}
	err2 = json.Unmarshal(buff, &passMap)
	if err != nil || err2 != nil {
		passMap["sanlab"] = "be4aacbd615bd8f2380d1baa5b6525f69686e3145bdb2948c9e8915d" //sanlab28660
	}
	go save(ctx)
}

func save(ctx context.Context) {
	wg, _ := ctx.Value("WaitGroup").(*sync.WaitGroup)
	wg.Add(1)
	defer wg.Done()
	select {
	case <-ctx.Done():

		bytes, err := json.Marshal(passMap)
		if err != nil {
			log.Println(err)
			return
		}
		log.Println("Save got DONE signal and writes")
		ioutil.WriteFile("./"+passwdFile, bytes, 0644)
	}
}

func (lhh *LilHttpHandler) setPassword(user, passwd string) {
	ar := sha256.Sum224([]byte(passwd))
	passMap[user] = hex.EncodeToString(ar[:])
}

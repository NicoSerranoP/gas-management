package controller

import (
	"encoding/json"
	"sync"
	"net/http/httputil"
	"net/http"
	"net/url"
	"io/ioutil"
	"bytes"
	"github.com/lacchain/gas-relay-signer/rpc"
	"github.com/lacchain/gas-relay-signer/model"
	"github.com/lacchain/gas-relay-signer/service"
	log "github.com/lacchain/gas-relay-signer/audit"
)

var lock sync.Mutex

//RelayController is the main controller
type RelayController struct {
	// The controller's configuration
	Config *model.Config
	RelaySignerService *service.RelaySignerService
}

//Init controller
func (controller *RelayController) Init(config *model.Config, relaySignerService *service.RelaySignerService){
	controller.Config = config
	controller.RelaySignerService = relaySignerService
}

//SignTransaction ...
func (controller *RelayController) SignTransaction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	log.GeneralLogger.Println("Body:", r.Body)

	buf, _ := ioutil.ReadAll(r.Body)
	rdr1 := ioutil.NopCloser(bytes.NewBuffer(buf))
	rdr2 := ioutil.NopCloser(bytes.NewBuffer(buf))

	log.GeneralLogger.Println("Request body : ", rdr1)

	var rpcMessage rpc.JsonrpcMessage

	err := json.NewDecoder(rdr1).Decode(&rpcMessage)
	if err != nil {
		log.GeneralLogger.Println("Invalid params")
		log.GeneralLogger.Println(err)
		return
	}

	log.GeneralLogger.Println("JSON-RPC Method:",rpcMessage.Method);

	if (rpcMessage.IsPrivTransaction()){
		r.Body=rdr2
		log.GeneralLogger.Println("Is a private Transaction, forward to Besu->Orion")

		serveReverseProxy(controller.Config.Application.NodeURL,w,r)
	}else if (rpcMessage.IsPrivRawTransaction()){
		r.Body=rdr2
		log.GeneralLogger.Println("Is a private send Transaction, decrease gas used")

		controller.RelaySignerService.DecreaseGasUsed(rpcMessage.ID)

		log.GeneralLogger.Println("forward to Besu->Orion")
		serveReverseProxy(controller.Config.Application.NodeURL,w,r)
	}else if (rpcMessage.IsRawTransaction()){
		processRawTransaction(controller.RelaySignerService,rpcMessage, w)
		return
	}else if (rpcMessage.IsGetTransactionReceipt()){
		processGetTransactionReceipt(controller.RelaySignerService,rpcMessage, w)
		return
	}else if(rpcMessage.IsGetTransactionCount()){
		processTransactionCount(controller.RelaySignerService,rpcMessage, w)
		return
	}else if(rpcMessage.IsGetBlockByNumber()){
		processGetBlockByNumber(controller.RelaySignerService,rpcMessage, w)
		return
	}else{
		r.Body=rdr2
		log.GeneralLogger.Println("Is another type of transaction, reverse proxy")
		
		serveReverseProxy(controller.Config.Application.NodeURL,w,r)
	}
}

func serveReverseProxy(target string, res http.ResponseWriter, req *http.Request) {
	// parse the url
	url, _ := url.Parse(target)

	// create the reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(url)

	// Update the headers to allow for SSL redirection
	req.URL.Host = url.Host
	req.URL.Scheme = url.Scheme
	req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
	req.Host = url.Host

	// Note that ServeHttp is non blocking and uses a go routine under the hood
	proxy.ServeHTTP(res, req)
}

func handleError(messageID json.RawMessage, err error) ([]byte) {
	log.GeneralLogger.Println(err)
	data, _ := json.Marshal(service.HandleError(messageID,err))
	
	return data
}
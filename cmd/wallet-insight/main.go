package main

import (
	"encoding/hex"
	"flag"
	"log"
	"math/big"
	"net/http"
	"time"

	"github.com/dynamicgo/config"
	"github.com/dynamicgo/slf4go"
	"github.com/gin-gonic/gin"
	ethrpc "github.com/inwecrypto/ethgo/rpc"
	neorpc "github.com/inwecrypto/neogo/rpc"
)

var logger = slf4go.Get("wallet-insight")

var configpath = flag.String("conf", "./wallet-insight.json", "wallet-insight config json")

var ethbalances map[string]map[string]string
var neobalances map[string]map[string]string

var ethclient *ethrpc.Client
var neoclient *neorpc.Client

var interval int64

func init() {
	neobalances = make(map[string]map[string]string)
	ethbalances = make(map[string]map[string]string)
}

type addressAsset struct {
	Address []string
	Asset   []string
}

type addressBalances struct {
	Address string
	Asset   string
	Value   string
}

func main() {

	flag.Parse()

	conf, err := config.NewFromFile(*configpath)

	if err != nil {
		logger.ErrorF("load wallet-insight config err , %s", err)
		return
	}

	ethclient = ethrpc.NewClient(conf.GetString("eth", ""))
	neoclient = neorpc.NewClient(conf.GetString("neo", ""))
	interval = conf.GetInt64("interval", 10)

	router := gin.New()
	router.Use(gin.Recovery())

	go ethmonitor()
	go neomonitor()

	router.POST("/getEthBalance", GetEthBalance)
	router.POST("/getNeoBalance", GetNeoBalance)

	router.Run(":8080")
}

func GetNeoBalance(c *gin.Context) {
	req := &addressAsset{}
	err := c.BindJSON(req)
	if err != nil {
		log.Println(err)
		c.JSON(http.StatusBadRequest, nil)
		return
	}

	rep := make([]addressBalances, 0)

	for k, v := range req.Address {
		address := v
		asset := req.Asset[k]

		res := addressBalances{}
		res.Address = address
		res.Asset = asset

		_, ok := neobalances[address]
		if !ok {
			neobalances[address] = make(map[string]string)
		}

		value, ok2 := neobalances[address][asset]
		if !ok2 {
			stat, err := neoclient.GetAccountState(address)
			if err != nil {
				log.Println("err:", err)
				continue
			}

			for _, v2 := range stat.Balances {
				if v2.Asset == asset {
					res.Value = v2.Value
				}

				neobalances[address][v2.Asset] = v2.Value
			}

		} else {
			res.Value = value
		}

		rep = append(rep, res)
	}

	c.JSON(http.StatusOK, rep)
}

func GetEthBalance(c *gin.Context) {

	req := &addressAsset{}

	err := c.BindJSON(req)
	if err != nil {
		log.Println(err)
		c.JSON(http.StatusBadRequest, nil)
		return
	}

	rep := make([]addressBalances, 0)

	for k, v := range req.Address {

		address := v
		asset := req.Asset[k]

		res := addressBalances{}
		res.Address = address
		res.Asset = asset

		_, ok := ethbalances[address]
		if !ok {
			ethbalances[address] = make(map[string]string)
		}

		value, ok2 := ethbalances[address][asset]
		if !ok2 {
			if asset == "eth" {
				value, err := ethclient.GetBalance(address)
				if err != nil {
					log.Println("err:", err)
					continue
				}

				ethbalances[address][asset] = hex.EncodeToString(((*big.Int)(value)).Bytes())
			} else {
				value, err := ethclient.GetTokenBalance(asset, address)
				if err != nil {
					log.Println("err:", err)
					continue
				}

				ethbalances[address][asset] = hex.EncodeToString(value.Bytes())
			}
			res.Value = ethbalances[address][asset]
		} else {
			res.Value = value
		}

		rep = append(rep, res)
	}

	c.JSON(http.StatusOK, rep)
}

func getAsset(address string, asset string) ([]*neorpc.UTXO, error) {
	return neoclient.GetBalance(address, asset)
}

func neomonitor() {
	tick := time.NewTicker(time.Duration(interval) * time.Second)
	for {
		select {
		case <-tick.C:
			for address, _ := range neobalances {
				stat, err := neoclient.GetAccountState(address)
				if err != nil {
					log.Println("err:", err)
					continue
				}

				for _, v2 := range stat.Balances {
					neobalances[address][v2.Asset] = v2.Value
				}
			}
		}
	}
}

func ethmonitor() {
	tick := time.NewTicker(time.Duration(interval) * time.Second)

	for {
		select {
		case <-tick.C:
			for address, _ := range ethbalances {
				for asset, _ := range ethbalances[address] {
					if asset == "eth" {
						value, err := ethclient.GetBalance(address)
						if err != nil {
							log.Println("err:", err)
							continue
						}

						ethbalances[address][asset] = hex.EncodeToString(((*big.Int)(value)).Bytes())
					} else {
						value, err := ethclient.GetTokenBalance(asset, address)
						if err != nil {
							log.Println("err:", err)
							continue
						}

						ethbalances[address][asset] = hex.EncodeToString(value.Bytes())
					}
				}
			}
		}
	}
}

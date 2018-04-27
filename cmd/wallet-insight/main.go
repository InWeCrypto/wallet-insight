package main

import (
	"encoding/hex"
	"flag"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dynamicgo/config"
	"github.com/dynamicgo/slf4go"
	"github.com/gin-gonic/gin"
	ethrpc "github.com/inwecrypto/ethgo/rpc"
	neorpc "github.com/inwecrypto/neogo/rpc"
)

var logger = slf4go.Get("wallet-insight")

var configpath = flag.String("conf", "./wallet-insight.json", "wallet-insight config json")

var ethbalances map[string]map[string]ETHCache
var neobalances map[string]*NEOCache
var ethmutex sync.Mutex
var neomutex sync.Mutex

var ethclient *ethrpc.Client
var neoclient *neorpc.Client

var interval int64
var keepTime int64

func init() {
	neobalances = make(map[string]*NEOCache)
	ethbalances = make(map[string]map[string]ETHCache)
}

type addressAsset struct {
	Address []string
	Asset   []string
}

type NEOCache struct {
	Time   int64
	Values map[string]string
}

type ETHCache struct {
	Value string
	Time  int64
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

	slf4go.SetLevel(slf4go.Warn)

	ethclient = ethrpc.NewClient(conf.GetString("eth", ""))
	neoclient = neorpc.NewClient(conf.GetString("neo", ""))
	interval = conf.GetInt64("interval", 10)
	keepTime = conf.GetInt64("keepTime", 600)

	router := gin.New()
	router.Use(gin.Recovery())

	go ethmonitor()
	go neomonitor()

	router.POST("/getEthBalance", GetEthBalance)
	router.POST("/getNeoBalance", GetNeoBalance)

	router.Run(":8000")
}

func GetNeoBalance(c *gin.Context) {
	req := &addressAsset{}
	err := c.BindJSON(req)
	if err != nil {
		logger.Error(err)
		c.JSON(http.StatusBadRequest, nil)
		return
	}

	rep := make([]*addressBalances, 0)

	neomutex.Lock()
	defer neomutex.Unlock()

	for k, v := range req.Address {
		address := v
		asset := req.Asset[k]

		res := &addressBalances{}
		res.Address = address
		res.Asset = asset
		res.Value = "0"
		rep = append(rep, res)

		cache, ok := neobalances[address]
		if !ok {
			cache = &NEOCache{
				Values: make(map[string]string),
			}
		}

		cache.Time = time.Now().Unix()

		value, ok2 := cache.Values[asset]

		if !ok2 {
			cache.Values[asset] = "0"

			stat, err := neoclient.GetAccountState(address)
			if err != nil {
				logger.Error(err)
				continue
			}

			for _, v2 := range stat.Balances {
				if v2.Asset == asset {
					res.Value = v2.Value
					break
				}
			}

			cache.Values[asset] = res.Value
		} else {
			res.Value = value
		}

		neobalances[address] = cache
	}

	c.JSON(http.StatusOK, rep)
}

func GetEthBalance(c *gin.Context) {

	req := &addressAsset{}

	err := c.BindJSON(req)
	if err != nil {
		logger.Error(err)
		c.JSON(http.StatusBadRequest, nil)
		return
	}

	rep := make([]*addressBalances, 0)

	ethmutex.Lock()
	defer ethmutex.Unlock()

	for k, v := range req.Address {

		address := strings.ToLower(v)
		asset := strings.ToLower(req.Asset[k])

		res := &addressBalances{}
		res.Address = address
		res.Asset = asset
		res.Value = "0x0"

		rep = append(rep, res)

		_, ok := ethbalances[address]
		if !ok {
			ethbalances[address] = make(map[string]ETHCache)
		}

		cache, ok2 := ethbalances[address][asset]

		cache.Time = time.Now().Unix()

		if !ok2 {
			cache.Value = "0x0"

			if asset == "eth" {
				value, err := ethclient.GetBalance(address)
				if err != nil {
					logger.Error(err)
					continue
				}

				res.Value = "0x" + hex.EncodeToString(((*big.Int)(value)).Bytes())

			} else {
				value, err := ethclient.GetTokenBalance(asset, address)
				if err != nil {
					logger.Error(err)
					continue
				}
				res.Value = "0x" + hex.EncodeToString(value.Bytes())
			}

			cache.Value = res.Value

		} else {
			res.Value = cache.Value
		}

		ethbalances[address][asset] = cache

	}

	c.JSON(http.StatusOK, rep)
}

func neomonitor() {
	tick := time.NewTicker(time.Duration(interval) * time.Second)
	for {
		select {
		case <-tick.C:

			neomutex.Lock()
			now := time.Now().Unix()

			dels := make([]string, 0)

			for address, cache := range neobalances {
				if keepTime+cache.Time < now {
					dels = append(dels, address)
					continue
				}

				stat, err := neoclient.GetAccountState(address)
				if err != nil {
					logger.Error(err)
					continue
				}

				for _, v2 := range stat.Balances {
					cache.Values[v2.Asset] = v2.Value
				}
			}

			for _, v := range dels {
				delete(neobalances, v)
			}

			neomutex.Unlock()
			logger.DebugF("neo len:%d", len(neobalances))
		}
	}
}

func ethmonitor() {
	tick := time.NewTicker(time.Duration(interval) * time.Second)

	for {
		select {
		case <-tick.C:
			ethmutex.Lock()

			now := time.Now().Unix()

			dels := make([]string, 0)

			for address, _ := range ethbalances {

				delAssets := make([]string, 0)

				for asset, cache := range ethbalances[address] {
					if keepTime+cache.Time < now {
						delAssets = append(delAssets, asset)
						continue
					}

					if asset == "eth" {
						value, err := ethclient.GetBalance(address)
						if err != nil {
							logger.Error(err)
							continue
						}

						cache.Value = "0x" + hex.EncodeToString(((*big.Int)(value)).Bytes())

						ethbalances[address][asset] = cache
					} else {
						value, err := ethclient.GetTokenBalance(asset, address)
						if err != nil {
							logger.Error(err)
							continue
						}

						cache.Value = "0x" + hex.EncodeToString(value.Bytes())

						ethbalances[address][asset] = cache
					}
				}

				for _, v := range delAssets {
					delete(ethbalances[address], v)
				}

				if len(ethbalances[address]) == 0 {
					dels = append(dels, address)
				}
			}

			for _, v := range dels {
				delete(ethbalances, v)
			}

			ethmutex.Unlock()
			logger.DebugF("eth len:%d", len(ethbalances))
		}
	}
}

package chassis

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/go-chassis/go-chassis/bootstrap"
	// highway package handles remote procedure calls
	_ "github.com/go-chassis/go-chassis/client/highway"
	_ "github.com/go-chassis/go-chassis/core/router/cse"
	_ "github.com/go-chassis/go-chassis/core/router/pilot"
	// rest package handle rest apis
	_ "github.com/go-chassis/go-chassis/client/rest"
	// archaius package to get the conguration info fron diffent configuration sources
	"github.com/go-chassis/go-chassis/core/common"
	"github.com/go-chassis/go-chassis/core/config"
	"github.com/go-chassis/go-chassis/core/handler"
	"github.com/go-chassis/go-chassis/core/lager"
	"github.com/go-chassis/go-chassis/core/loadbalancer"
	"github.com/go-chassis/go-chassis/core/registry"
	// archaius panel
	_ "github.com/go-chassis/go-chassis/control/archaius"
	// file package for file based registration
	_ "github.com/go-chassis/go-chassis/core/registry/file"
	// servicecenter package handles service center api calls
	_ "github.com/go-chassis/go-chassis/core/registry/servicecenter"
	// pilot package handles istio pilot SDS api calls
	_ "github.com/go-chassis/go-chassis/core/registry/pilot"
	"github.com/go-chassis/go-chassis/core/router"
	"github.com/go-chassis/go-chassis/core/server"
	"github.com/go-chassis/go-chassis/core/tracing"
	"github.com/go-chassis/go-chassis/eventlistener"
	// metric plugin
	_ "github.com/go-chassis/go-chassis/metrics/prom"
	// aes package handles security related plugins
	_ "github.com/go-chassis/go-chassis/security/plugins/aes"
	_ "github.com/go-chassis/go-chassis/security/plugins/plain"
	_ "github.com/go-chassis/go-chassis/server/restful"
	// highway package register the highway server plugin
	_ "github.com/go-chassis/go-chassis/server/highway"
	// import config center plugins
	_ "github.com/go-chassis/go-cc-client/apollo-client"
	_ "github.com/go-chassis/go-cc-client/configcenter-client"
	"github.com/go-chassis/go-chassis/config-center"
	"github.com/go-chassis/go-chassis/control"
	"github.com/go-chassis/go-chassis/core/archaius"
	"github.com/go-chassis/go-chassis/core/metadata"
	"github.com/go-chassis/go-chassis/metrics"
	"github.com/go-chassis/go-chassis/pkg/runtime"
)

var goChassis *chassis

func init() {
	goChassis = &chassis{}
}

type chassis struct {
	version     string
	schemas     []*Schema
	mu          sync.Mutex
	Initialized bool

	DefaultConsumerChainNames map[string]string
	DefaultProviderChainNames map[string]string
}

// Schema struct for to represent schema info
type Schema struct {
	serverName string
	schema     interface{}
	opts       []server.RegisterOption
}

func (c *chassis) initChains(chainType string) error {
	var defaultChainName = "default"
	var handlerNameMap = map[string]string{defaultChainName: ""}
	switch chainType {
	case common.Provider:
		if len(config.GlobalDefinition.Cse.Handler.Chain.Provider) != 0 {
			handlerNameMap = config.GlobalDefinition.Cse.Handler.Chain.Provider
		} else {
			handlerNameMap = c.DefaultProviderChainNames
		}
	case common.Consumer:
		if len(config.GlobalDefinition.Cse.Handler.Chain.Consumer) != 0 {
			handlerNameMap = config.GlobalDefinition.Cse.Handler.Chain.Consumer
		} else {
			handlerNameMap = c.DefaultConsumerChainNames
		}
	}
	lager.Logger.Debugf("Init %s's handlermap", chainType)
	return handler.CreateChains(chainType, handlerNameMap)
}
func (c *chassis) initHandler() error {
	if err := c.initChains(common.Provider); err != nil {
		lager.Logger.Errorf(err, "chain int failed")
		return err
	}
	if err := c.initChains(common.Consumer); err != nil {
		lager.Logger.Errorf(err, "chain int failed")
		return err
	}
	lager.Logger.Info("Chain init success")
	return nil
}

//Init
func (c *chassis) initialize() error {
	if c.Initialized {
		return nil
	}
	err := config.Init()
	if err != nil {
		lager.Logger.Error("Failed to initialize conf,", err)
		return err
	}
	if err := runtime.Init(); err != nil {
		return err
	}
	if err := control.Init(); err != nil {
		return err
	}
	err = c.initHandler()
	if err != nil {
		lager.Logger.Errorf(err, "Handler init failed")
		return err
	}

	err = server.Init()
	if err != nil {
		return err
	}
	bootstrap.Bootstrap()
	if archaius.GetBool("cse.service.registry.disabled", false) != true {
		err := registry.Enable()
		if err != nil {
			return err
		}
		if err := loadbalancer.Enable(); err != nil {
			return err
		}
	}

	configcenter.InitConfigCenter()
	// router needs get configs from config-center when init
	// so it must init after bootstrap
	if err = router.Init(); err != nil {
		return err
	}

	if err = tracing.Init(); err != nil {
		return err
	}
	if err = metrics.Init(); err != nil {
		return err
	}
	eventlistener.Init()
	c.Initialized = true
	return nil
}

func (c *chassis) registerSchema(serverName string, structPtr interface{}, opts ...server.RegisterOption) {
	schema := &Schema{
		serverName: serverName,
		schema:     structPtr,
		opts:       opts,
	}
	c.mu.Lock()
	c.schemas = append(c.schemas, schema)
	c.mu.Unlock()
}

func (c *chassis) start() error {
	if !c.Initialized {
		return fmt.Errorf("the chassis do not init. please run chassis.Init() first")
	}

	for _, v := range c.schemas {
		if v == nil {
			continue
		}
		s, err := server.GetServer(v.serverName)
		if err != nil {
			return err
		}
		_, err = s.Register(v.schema, v.opts...)
		if err != nil {
			return err
		}
	}
	err := server.StartServer()
	if err != nil {
		return err
	}
	return nil
}

//RegisterSchema Register a API service to specific server by name
//You must register API first before Call Init
func RegisterSchema(serverName string, structPtr interface{}, opts ...server.RegisterOption) {
	goChassis.registerSchema(serverName, structPtr, opts...)
}

//SetDefaultConsumerChains your custom chain map for Consumer,if there is no config, this default chain will take affect
func SetDefaultConsumerChains(c map[string]string) {
	goChassis.DefaultConsumerChainNames = c
}

//SetDefaultProviderChains set your custom chain map for Provider,if there is no config, this default chain will take affect
func SetDefaultProviderChains(c map[string]string) {
	goChassis.DefaultProviderChainNames = c
}

//Run bring up the service,it will not return error,instead just waiting for os signal,and shutdown gracefully
func Run() {
	err := goChassis.start()
	if err != nil {
		lager.Logger.Error("run chassis fail:", err)
	}
	if !config.GetRegistratorDisable() {
		//Register instance after Server started
		if err := registry.DoRegister(); err != nil {
			lager.Logger.Error("register instance fail:", err)
		}
	}
	//Graceful shutdown
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGILL, syscall.SIGTRAP, syscall.SIGABRT)
	select {
	case s := <-c:
		lager.Logger.Info("got os signal " + s.String())
	case err := <-server.ErrRuntime:
		lager.Logger.Info("got Server Error " + err.Error())
	}
	for name, s := range server.GetServers() {
		lager.Logger.Info("stopping server " + name + "...")
		err := s.Stop()
		if err != nil {
			lager.Logger.Errorf(err, "servers failed to stop")
		}
		lager.Logger.Info(name + " server stop success")
	}
	if !config.GetRegistratorDisable() {
		if err = server.UnRegistrySelfInstances(); err != nil {
			lager.Logger.Errorf(err, "servers failed to unregister")
		}
	}

}

//Init prepare the chassis framework runtime
func Init() error {
	if goChassis.DefaultConsumerChainNames == nil {
		defaultChain := strings.Join([]string{
			handler.RatelimiterConsumer,
			handler.BizkeeperConsumer,
			handler.Router,
			handler.Loadbalance,
			handler.TracingConsumer,
			handler.Transport,
		}, ",")
		goChassis.DefaultConsumerChainNames = map[string]string{
			common.DefaultKey: defaultChain,
		}
	}
	if goChassis.DefaultProviderChainNames == nil {
		defaultChain := strings.Join([]string{
			handler.RatelimiterProvider,
			handler.TracingProvider,
			handler.BizkeeperProvider,
		}, ",")
		goChassis.DefaultProviderChainNames = map[string]string{
			common.DefaultKey: defaultChain,
		}
	}
	if err := goChassis.initialize(); err != nil {
		log.Println("Init chassis fail:", err)
		return err
	}
	lager.Logger.Infof("Init chassis success, Version is %s", metadata.SdkVersion)
	return nil
}

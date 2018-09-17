package istio

import (
	"crypto/tls"
	"fmt"
	"github.com/go-chassis/go-chassis/control"
	"github.com/go-chassis/go-chassis/core/config"
	"github.com/go-chassis/go-chassis/core/config/model"
	"github.com/go-chassis/go-chassis/core/invocation"
	chassisTLS "github.com/go-chassis/go-chassis/core/tls"
	"github.com/go-chassis/go-chassis/pkg/istio/client"
	"github.com/go-chassis/go-chassis/pkg/util/iputil"
	"github.com/go-chassis/go-chassis/third_party/forked/afex/hystrix-go/hystrix"
	"strings"
)

func newPanel(options control.Options) control.Panel {
	return &Panel{}
}

// RouterTLS defines tls prefix
const RouterTLS = "router"

//Panel pull configs from Pilot
type Panel struct {
}

func (panel *Panel) GetCircuitBreaker(inv invocation.Invocation, serviceType string) (string, hystrix.CommandConfig) {

	return "", hystrix.CommandConfig{}
}
func (panel *Panel) GetLoadBalancing(inv invocation.Invocation) control.LoadBalancingConfig {

	clusterconfigs, _ := getClusterConfigFromPilot()

	fmt.Println(clusterconfigs)
	return control.LoadBalancingConfig{}

}
func (panel *Panel) GetRateLimiting(inv invocation.Invocation, serviceType string) control.RateLimitingConfig {
	return control.RateLimitingConfig{}
}
func (panel *Panel) GetFaultInjection(inv invocation.Invocation) model.Fault {
	return model.Fault{}
}
func (panel *Panel) GetEgressRule(inv invocation.Invocation) {

}

func init() {
	control.InstallPlugin("istio", newPanel)
}

// Options defines how to init router and its fetcher
type Options struct {
	Endpoints []string
	EnableSSL bool
	TLSConfig *tls.Config
	Version   string

	//TODO: need timeout for client
	// TimeOut time.Duration
}

// ToPilotOptions translate options to client options
func (o Options) ToPilotOptions() *client.PilotOptions {
	return &client.PilotOptions{Endpoints: o.Endpoints}
}

// get Cluster Config from pilot
func getClusterConfigFromPilot() (*model.LoadBalancingConfig, error) {

	optionss, _ := getSpecifiedOptions()

	grpcClient, err := client.NewGRPCPilotClient(optionss.ToPilotOptions())
	if err != nil {
		return nil, fmt.Errorf("connect to pilot failed: %v", err)
	}
	lbRules := &model.LoadBalancingConfig{}
	clusterConfigs, err := grpcClient.GetAllClusterConfigurations()
	fmt.Println("Error from getClusterConfigFromPilot", err)
	fmt.Println(clusterConfigs)
	return lbRules, nil
}

func getSpecifiedOptions() (opts Options, err error) {
	hosts, scheme, err := iputil.URIs2Hosts(strings.Split(config.GetRouterEndpoints(), ","))
	if err != nil {
		return
	}
	opts.Endpoints = hosts
	// TODO: envoy api v1 or v2
	// opts.Version = config.GetRouterAPIVersion()
	opts.TLSConfig, err = chassisTLS.GetTLSConfig(scheme, RouterTLS)
	if err != nil {
		return
	}
	if opts.TLSConfig != nil {
		opts.EnableSSL = true
	}
	return
}

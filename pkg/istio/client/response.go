package client

import (
	"errors"

	xdsapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/go-chassis/go-chassis/pkg/istio/util"
	"fmt"
)

// GetRouteConfiguration returns routeconfiguration from discovery response
func GetRouteConfiguration(res *xdsapi.DiscoveryResponse) (*xdsapi.RouteConfiguration, error) {
	if res.TypeUrl != util.RouteType || res.Resources[0].TypeUrl != util.RouteType {
		return nil, errors.New("Invalid typeURL" + res.TypeUrl)
	}

	cla := &xdsapi.RouteConfiguration{}
	err := cla.Unmarshal(res.Resources[0].Value)
	if err != nil {
		return nil, err
	}
	return cla, nil
}

func GetClusterConfigurations(res *xdsapi.DiscoveryResponse) (*xdsapi.Cluster, error){
	cla := &xdsapi.Cluster{}
	fmt.Println("Before Unmarshall : ", cla)
	err := cla.Unmarshal(res.Resources[0].Value)
	if err != nil {
		return nil, err
	}
	fmt.Println(cla)
	return cla, nil
}

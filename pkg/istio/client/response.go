package client

import (
	"errors"

	xdsapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/go-chassis/go-chassis/pkg/istio/util"
	"fmt"
	"github.com/gogo/protobuf/proto"
	"github.com/go-chassis/go-chassis/core/lager"
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

func GetClusterConfigurations(resp *xdsapi.DiscoveryResponse) ([]xdsapi.Cluster, error){
	resources := resp.GetResources()

	var cluster xdsapi.Cluster
	clusters := []xdsapi.Cluster{}
	for _, res := range resources {
		if err := proto.Unmarshal(res.GetValue(), &cluster); err != nil {
			lager.Logger.Warnf("Failed to unmarshal cluster resource: %s", err.Error())
		} else {
			clusters = append(clusters, cluster)
		}
	}

	fmt.Println(clusters)

	return clusters, nil
}

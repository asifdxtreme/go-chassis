package client

import (
	"context"
	"fmt"
	"time"

	apiv2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	apiv2core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoy_api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	xds "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/go-chassis/go-chassis/pkg/istio/util"
	"google.golang.org/grpc"
)

// PilotClient is a interface for the client to communicate to pilot
type PilotClient interface {
	RDS

	// TODO: add all xDS interface
	EDS
	CDS
	LDS
}

// RDS defines route discovery service interface
type RDS interface {
	GetAllRouteConfigurations() (*envoy_api.RouteConfiguration, error)
	GetRouteConfigurationsByPort(string) (*envoy_api.RouteConfiguration, error)
}

// EDS defines endpoint discovery service interface
type EDS interface{}

// CDS defines cluster discovery service interface
type CDS interface{
	GetAllClusterConfigurations()(*envoy_api.Cluster, error)
	//GetClusterConfigurationsByClusterID()(*envoy_api.Cluster, error)
}

// LDS defines listener discovery service interface
type LDS interface{}

type pilotClient struct {
	rawConn *grpc.ClientConn

	adsConn xds.AggregatedDiscoveryServiceClient
	edsConn envoy_api.EndpointDiscoveryServiceClient
	cdsConn xds.AggregatedDiscoveryServiceClient
}

// NewGRPCPilotClient returns new PilotClient from options
func NewGRPCPilotClient(cfg *PilotOptions) (PilotClient, error) {
	// TODO: credentials need to be added here
	// set dial options from config

	conn, err := grpc.Dial(cfg.Endpoints[0], grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("new grpc pilot client error: %v", err)
	}
	ads := xds.NewAggregatedDiscoveryServiceClient(conn)
	eds := envoy_api.NewEndpointDiscoveryServiceClient(conn)

	return &pilotClient{rawConn: conn,
		adsConn: ads, edsConn: eds,
	}, nil
}

func NewXdsClient(pilotAddr string, nodeInfo *NodeInfo) (*XdsClient, error) {
	// TODO Handle the array
	xdsClient := &XdsClient{
		PilotAddr: pilotAddr,
		nodeInfo:  nodeInfo,
	}
	xdsClient.NodeID = fmt.Sprintf("sidecar~%s~%s~%s", nodeInfo.InstanceIP, nodeInfo.PodName, nodeInfo.Namespace)
	xdsClient.NodeCluster = nodeInfo.PodName

	xdsClient.ReqCaches = map[XdsType]*XdsReqCache{
		TypeCds: &XdsReqCache{},
		TypeEds: &XdsReqCache{},
		TypeLds: &XdsReqCache{},
		TypeRds: &XdsReqCache{},
	}
	return xdsClient, nil
}

func (c *pilotClient) GetAllClusterConfigurations ()(*envoy_api.Cluster, error) {

	cds, err := c.cdsConn.StreamAggregatedResources(context.Background())

	if err != nil {
		return nil, fmt.Errorf("[CDS] stream error: %v", err)
	}
	nodeID := util.BuildNodeID()
	cds.Send(&envoy_api.DiscoveryRequest{
		ResponseNonce: time.Now().String(),
		Node: &envoy_api_core.Node{
			Id: nodeID,
		},
		ResourceNames: []string{},
		TypeUrl:       "type.googleapis.com/envoy.api.v2.Cluster"
	})

	res, err := cds.Recv()
	if err != nil {
		return nil, fmt.Errorf("[RDS] recv error for %s(%s): %v", util.RDSHttpProxy, nodeID, err)
	}
	clusters, _ := GetClusterConfigurations(res)
	return clusters[0], nil


	/*client, _ := NewXdsClient("", nil)
	adsResClient, conn, err := getAdsResClient(client)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	req := &apiv2.DiscoveryRequest{
		TypeUrl:       "type.googleapis.com/envoy.api.v2.Cluster",
		VersionInfo:   client.getVersionInfo(TypeCds),
		ResponseNonce: client.getNonce(TypeCds),
	}
	req.Node = &apiv2core.Node{
		// Sample taken from istio: router~172.30.77.6~istio-egressgateway-84b4d947cd-rqt45.istio-system~istio-system.svc.cluster.local-2
		// The Node.Id should be in format {nodeType}~{ipAddr}~{serviceId~{domain}, splitted by '~'
		// The format is required by pilot
		Id:      client.NodeID,
		Cluster: client.NodeCluster,
	}
	if err := adsResClient.Send(req); err != nil {
		return nil, err
	}
	resp, err := adsResClient.Recv()
	if err != nil {
		return nil, err
	}
	client.setNonce(TypeCds, resp.GetNonce())
	client.setVersionInfo(TypeCds, resp.GetVersionInfo())
	resources := resp.GetResources()
	var cluster apiv2.Cluster
	clusters := []apiv2.Cluster{}
	for _, res := range resources {
		if err := proto.Unmarshal(res.GetValue(), &cluster); err != nil {
			lager.Logger.Warnf("Failed to unmarshal cluster resource: %s", err.Error())
		} else {
			clusters = append(clusters, cluster)
		}
	}
	return clusters, nil*/

}

func getAdsResClient(client *XdsClient) (v2.AggregatedDiscoveryService_StreamAggregatedResourcesClient, *grpc.ClientConn, error) {
	conn, err := client.getGrpcConn()
	if err != nil {
		return nil, nil, err
	}

	adsClient := v2.NewAggregatedDiscoveryServiceClient(conn)
	adsResClient, err := adsClient.StreamAggregatedResources(context.Background())
	if err != nil {
		return nil, nil, err
	}

	return adsResClient, conn, nil
}

func (c *pilotClient) GetAllRouteConfigurations() (*envoy_api.RouteConfiguration, error) {
	// TODO: this RDS stream can be reuse in all RDS request?
	rds, err := c.adsConn.StreamAggregatedResources(context.Background())
	if err != nil {
		return nil, fmt.Errorf("[RDS] stream error: %v", err)
	}

	nodeID := util.BuildNodeID()
	err = rds.Send(&envoy_api.DiscoveryRequest{
		ResponseNonce: time.Now().String(),
		Node: &envoy_api_core.Node{
			Id: nodeID,
		},
		ResourceNames: []string{util.RDSHttpProxy},
		TypeUrl:       util.RouteType})
	if err != nil {
		return nil, fmt.Errorf("[RDS] send req error for %s(%s): %v", util.RDSHttpProxy, nodeID, err)
	}

	res, err := rds.Recv()
	if err != nil {
		return nil, fmt.Errorf("[RDS] recv error for %s(%s): %v", util.RDSHttpProxy, nodeID, err)
	}
	return GetRouteConfiguration(res)
}

func (c *pilotClient) GetRouteConfigurationsByPort(port string) (*envoy_api.RouteConfiguration, error) {
	// TODO: this RDS stream can be reuse in all RDS request?
	rds, err := c.adsConn.StreamAggregatedResources(context.Background())
	if err != nil {
		return nil, fmt.Errorf("[RDS] stream error: %v", err)
	}

	nodeID := util.BuildNodeID()
	err = rds.Send(&envoy_api.DiscoveryRequest{
		ResponseNonce: time.Now().String(),
		Node: &envoy_api_core.Node{
			Id: nodeID,
		},
		ResourceNames: []string{port},
		TypeUrl:       util.RouteType})
	if err != nil {
		return nil, fmt.Errorf("[RDS] send req error for %s(%s): %v", util.RDSHttpProxy, nodeID, err)
	}

	res, err := rds.Recv()
	if err != nil {
		return nil, fmt.Errorf("[RDS] recv error for %s(%s): %v", util.RDSHttpProxy, nodeID, err)
	}
	return GetRouteConfiguration(res)
}

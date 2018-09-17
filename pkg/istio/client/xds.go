package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"

	apiv2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	apiv2core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	apiv2route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/go-chassis/go-chassis/core/lager"
	"github.com/gogo/protobuf/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type XdsClient struct {
	PilotAddr   string
	TlsConfig   *tls.Config
	ReqCaches   map[XdsType]*XdsReqCache
	nodeInfo    *NodeInfo
	NodeID      string
	NodeCluster string
}

type XdsType string

const (
	TypeCds XdsType = "cds"
	TypeEds XdsType = "eds"
	TypeLds XdsType = "lds"
	TypeRds XdsType = "rds"
)

type XdsReqCache struct {
	Nonce       string
	VersionInfo string
}

type NodeInfo struct {
	PodName    string
	Namespace  string
	InstanceIP string
}

type XdsClusterInfo struct {
	ClusterName  string
	Direction    string
	Port         string
	Subset       string
	HostName     string
	ServiceName  string
	Namespace    string
	DomainSuffix string // DomainSuffix might not be used
	Tags         map[string]string
	Addrs        []string // The accessible addresses of the endpoints
}

/*func NewXdsClient(pilotAddr string, nodeInfo *NodeInfo) (*XdsClient, error) {
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
}*/

func (client *XdsClient) getGrpcConn() (*grpc.ClientConn, error) {
	var conn *grpc.ClientConn
	var err error
	if client.TlsConfig != nil {
		creds := credentials.NewTLS(client.TlsConfig)
		conn, err = grpc.Dial(client.PilotAddr, grpc.WithTransportCredentials(creds))
	} else {
		conn, err = grpc.Dial(client.PilotAddr, grpc.WithInsecure())
	}

	return conn, err
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

func (client *XdsClient) getRouterClusters(clusterName string) ([]string, error) {
	virtualHosts, err := client.RDS(clusterName)
	if err != nil {
		return nil, err
	}

	routerClusters := []string{}
	for _, h := range virtualHosts {
		for _, r := range h.Routes {
			routerClusters = append(routerClusters, r.GetRoute().GetCluster())
		}
	}

	return routerClusters, nil
}

func (client *XdsClient) getVersionInfo(resType XdsType) string {
	return client.ReqCaches[resType].VersionInfo
}
func (client *XdsClient) getNonce(resType XdsType) string {
	return client.ReqCaches[resType].Nonce
}

func (client *XdsClient) setVersionInfo(resType XdsType, versionInfo string) {
	client.ReqCaches[resType].VersionInfo = versionInfo
}

func (client *XdsClient) setNonce(resType XdsType, nonce string) {
	client.ReqCaches[resType].Nonce = nonce
}

func (client *XdsClient) CDS() ([]apiv2.Cluster, error) {
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
	/*
	Check LBConfig from cluster
	 */
	 for _, cl := range clusters {
	 	checkLbConfiguration := cl.GetLbConfig()

	 }
	 //====================
	return clusters, nil
}

func (client *XdsClient) EDS(clusterName string) (*apiv2.ClusterLoadAssignment, error) {
	adsResClient, conn, err := getAdsResClient(client)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	req := &apiv2.DiscoveryRequest{
		TypeUrl:       "type.googleapis.com/envoy.api.v2.ClusterLoadAssignment",
		VersionInfo:   client.getVersionInfo(TypeEds),
		ResponseNonce: client.getNonce(TypeEds),
	}

	req.Node = &apiv2core.Node{
		Id:      client.NodeID,
		Cluster: client.NodeCluster,
	}
	req.ResourceNames = []string{clusterName}
	if err := adsResClient.Send(req); err != nil {
		return nil, err
	}

	resp, err := adsResClient.Recv()
	if err != nil {
		return nil, err
	}

	resources := resp.GetResources()
	client.setNonce(TypeEds, resp.GetNonce())
	client.setVersionInfo(TypeEds, resp.GetVersionInfo())

	var loadAssignment apiv2.ClusterLoadAssignment
	var e error
	// endpoints := []apiv2.ClusterLoadAssignment{}

	for _, res := range resources {
		if err := proto.Unmarshal(res.GetValue(), &loadAssignment); err != nil {
			e = err
		} else {
			// The cluster's LoadAssignment will always be ONE, with Endpoints as its field
			break
		}
	}
	return &loadAssignment, e
}
func (client *XdsClient) RDS(clusterName string) ([]apiv2route.VirtualHost, error) {
	clusterInfo := ParseClusterName(clusterName)
	if clusterInfo == nil {
		return nil, fmt.Errorf("Invalid clusterName for routers: %s", clusterName)
	}

	adsResClient, conn, err := getAdsResClient(client)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	req := &apiv2.DiscoveryRequest{
		TypeUrl:       "type.googleapis.com/envoy.api.v2.RouteConfiguration",
		VersionInfo:   client.getVersionInfo(TypeRds),
		ResponseNonce: client.getNonce(TypeRds),
	}

	req.Node = &apiv2core.Node{
		Id:      client.NodeID,
		Cluster: client.NodeCluster,
	}
	req.ResourceNames = []string{clusterName}
	if err := adsResClient.Send(req); err != nil {
		return nil, err
	}

	resp, err := adsResClient.Recv()
	if err != nil {
		return nil, err
	}

	resources := resp.GetResources()
	client.setNonce(TypeRds, resp.GetNonce())
	client.setVersionInfo(TypeRds, resp.GetVersionInfo())

	var route apiv2.RouteConfiguration
	virtualHosts := []apiv2route.VirtualHost{}

	for _, res := range resources {
		if err := proto.Unmarshal(res.GetValue(), &route); err != nil {
			lager.Logger.Warnf("Failed to unmarshal router resource: ", err.Error())
		} else {
			vhosts := route.GetVirtualHosts()
			for _, vhost := range vhosts {
				if vhost.Name == fmt.Sprintf("%s:%s", clusterInfo.ServiceName, clusterInfo.Port) {
					virtualHosts = append(virtualHosts, vhost)
				}
			}
		}
	}
	return virtualHosts, nil
}

func (client *XdsClient) LDS() ([]apiv2.Listener, error) {
	adsResClient, conn, err := getAdsResClient(client)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	req := &apiv2.DiscoveryRequest{
		TypeUrl:       "type.googleapis.com/envoy.api.v2.ClusterLoadAssignment",
		VersionInfo:   client.getVersionInfo(TypeLds),
		ResponseNonce: client.getNonce(TypeLds),
	}

	req.Node = &apiv2core.Node{
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

	resources := resp.GetResources()
	client.setNonce(TypeLds, resp.GetNonce())
	client.setVersionInfo(TypeLds, resp.GetVersionInfo())

	var listener apiv2.Listener
	listeners := []apiv2.Listener{}

	for _, res := range resources {
		if err := proto.Unmarshal(res.GetValue(), &listener); err != nil {
			lager.Logger.Warnf("Failed to unmarshal listener resource: ", err.Error())
		} else {
			listeners = append(listeners, listener)
		}
	}
	return listeners, nil
}

func ParseClusterName(clusterName string) *XdsClusterInfo {
	// clusterName format: |direction|port|subset|hostName|
	// hostName format: |svc.namespace.svc.cluster.local

	parts := strings.Split(clusterName, "|")
	if len(parts) != 4 {
		return nil
	}

	hostnameParts := strings.Split(parts[3], ".")
	if len(hostnameParts) < 2 {
		return nil
	}

	cluster := &XdsClusterInfo{
		Direction:    parts[0],
		Port:         parts[1],
		Subset:       parts[2],
		HostName:     parts[3],
		ServiceName:  hostnameParts[0],
		Namespace:    hostnameParts[1],
		DomainSuffix: strings.Join(hostnameParts[2:], "."),
		ClusterName:  clusterName,
	}

	return cluster
}
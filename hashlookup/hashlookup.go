package hashlookup

import (
    "context"
    "encoding/json"
    "errors"

    "github.com/libp2p/go-libp2p-core/host"
    "github.com/libp2p/go-libp2p-discovery"

    "github.com/Multi-Tier-Cloud/common/p2pnode"
    "github.com/Multi-Tier-Cloud/hash-lookup/hl-common"
)

func GetHash(query string) (contentHash, dockerHash string, err error) {
    ctx := context.Background()
    nodeConfig := p2pnode.NewConfig()
    node, err := p2pnode.NewNode(ctx, nodeConfig)
    if err != nil {
        return "", "", err
    }
    defer node.Host.Close()
    defer node.DHT.Close()

    contentHash, dockerHash, err = GetHashWithHostRouting(ctx, node.Host,
        node.RoutingDiscovery, query)
    return contentHash, dockerHash, err
}

func GetHashWithHostRouting(
    ctx context.Context, host host.Host,
    routingDiscovery *discovery.RoutingDiscovery, query string) (
    contentHash, dockerHash string, err error) {
    
    response, err := common.SendRequestWithHostRouting(
        ctx, host, routingDiscovery, common.LookupProtocolID, []byte(query))
    if err != nil {
        return "", "", err
    }

    var respInfo common.LookupResponse
    err = json.Unmarshal(response, &respInfo)
    if err != nil {
        return "", "", err
    }

    err = nil
    if !respInfo.LookupOk {
        err = errors.New("hashlookup: Error finding hash for " + query)
    }
    return respInfo.ContentHash, respInfo.DockerHash, err
}

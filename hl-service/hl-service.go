package main

import (
    "context"
    "encoding/json"
    "flag"
    "fmt"
    "os"
    "os/exec"
    "strconv"
    "strings"
    "time"

    "github.com/libp2p/go-libp2p-core/network"
    "github.com/libp2p/go-libp2p-core/protocol"
    
    "go.etcd.io/etcd/clientv3"

    "github.com/Multi-Tier-Cloud/common/p2pnode"
    "github.com/Multi-Tier-Cloud/common/p2putil"
    "github.com/Multi-Tier-Cloud/hash-lookup/hl-common"
)

type etcdData struct {
    ContentHash string
    DockerHash string
}

func main() {
    newEtcdClusterFlag := flag.Bool("new-etcd-cluster", false,
        "Start running new etcd cluster")
    etcdIpFlag := flag.String("etcd-ip", "127.0.0.1",
        "Local etcd instance IP address")
    etcdClientPortFlag := flag.Int("etcd-client-port", 2379,
        "Local etcd instance client port")
    etcdPeerPortFlag := flag.Int("etcd-peer-port", 2380,
        "Local etcd instance peer port")
    localFlag := flag.Bool("local", false,
        "For debugging: Run locally and do not connect to bootstrap peers\n" +
        "(this option overrides the '--bootstrap' flag)")
    bootstrapFlag := flag.String("bootstrap", "",
        "For debugging: Connect to specified bootstrap node multiaddress")
    flag.Parse()

    ctx := context.Background()

    etcdClientEndpoint := *etcdIpFlag + ":" + strconv.Itoa(*etcdClientPortFlag)
    etcdPeerEndpoint := *etcdIpFlag + ":" + strconv.Itoa(*etcdPeerPortFlag)

    etcdClientUrl := "http://" + etcdClientEndpoint
    etcdPeerUrl := "http://" + etcdPeerEndpoint

    etcdName := fmt.Sprintf(
        "%s-%d-%d", *etcdIpFlag, *etcdClientPortFlag, *etcdPeerPortFlag)

    initialCluster := etcdName + "=" + etcdPeerUrl
    clusterState := "new"

    var err error
    
    if !(*newEtcdClusterFlag) {
        initialCluster, err = sendMemberAddRequest(
            etcdName, etcdPeerUrl, *localFlag, *bootstrapFlag)
        if err != nil {
            panic(err)
        }
        clusterState = "existing"
    }

    etcdArgs := []string{
        "--name", etcdName,
        "--listen-client-urls", etcdClientUrl,
        "--advertise-client-urls", etcdClientUrl,
        "--listen-peer-urls", etcdPeerUrl,
        "--initial-advertise-peer-urls", etcdPeerUrl,
        "--initial-cluster", initialCluster,
        "--initial-cluster-state", clusterState,
    }
    fmt.Println(etcdArgs)
    
    cmd := exec.Command("etcd", etcdArgs...)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    go func() {
        err := cmd.Run()
        if err != nil {
            panic(err)
        }
    }()

    etcdCli, err := clientv3.New(clientv3.Config{
        Endpoints: []string{etcdClientEndpoint},
        DialTimeout: 5 * time.Second,
    })
    if err != nil {
        panic(err)
    }
    defer etcdCli.Close()

    testData := etcdData{"general", "kenobi"}
    testDataBytes, err := json.Marshal(testData)
    if err != nil {
        panic(err)
        return
    }
    putResp, err := etcdCli.Put(ctx, "hello-there", string(testDataBytes))
    if err != nil {
        panic(err)
    }
    fmt.Println("etcd Response:", putResp)

    nodeConfig := p2pnode.NewConfig()
    if *localFlag {
        nodeConfig.BootstrapPeers = []string{}
    } else if *bootstrapFlag != "" {
        nodeConfig.BootstrapPeers = []string{*bootstrapFlag}
    }
    nodeConfig.StreamHandlers = append(nodeConfig.StreamHandlers,
        handleLookup(etcdCli), handleList(etcdCli), handleAdd(etcdCli),
        handleMemberAdd(etcdCli))
    nodeConfig.HandlerProtocolIDs = append(nodeConfig.HandlerProtocolIDs,
        common.LookupProtocolID, common.ListProtocolID, common.AddProtocolID,
        memberAddProtocolID)
    nodeConfig.Rendezvous = append(nodeConfig.Rendezvous,
        common.HashLookupRendezvousString)
    node, err := p2pnode.NewNode(ctx, nodeConfig)
    if err != nil {
        if *localFlag && err.Error() == "Failed to connect to any bootstraps" {
            fmt.Println("Local run, not connecting to bootstraps")
        } else {
            panic(err)
        }
    }
    defer node.Host.Close()
    defer node.DHT.Close()

    // fmt.Println("Host ID:", node.Host.ID())
    // fmt.Println("Listening on:", node.Host.Addrs())
    
    fmt.Println("Waiting to serve connections...")

    select {}
}

func handleLookup(etcdCli *clientv3.Client) func(network.Stream) {
    return func(stream network.Stream) {
        data, err := p2putil.ReadMsg(stream)
        if err != nil {
            fmt.Println(err)
            return
        }
        
        reqStr := strings.TrimSpace(string(data))
        fmt.Println("Lookup request:", reqStr)

        contentHash, dockerHash, ok, err := lookupServiceHash(etcdCli, reqStr)
        if err != nil {
            fmt.Println(err)
            stream.Reset()
            return
        }

        respInfo := common.LookupResponse{contentHash, dockerHash, ok}
        respBytes, err := json.Marshal(respInfo)
        if err != nil {
            fmt.Println(err)
            stream.Reset()
            return
        }

        fmt.Println("Lookup response: ", string(respBytes))

        err = p2putil.WriteMsg(stream, respBytes)
        if err != nil {
            fmt.Println(err)
            return
        }
    }
}

func handleList(etcdCli *clientv3.Client) func(network.Stream) {
    return func(stream network.Stream) {
        fmt.Println("List request")

        contentHashes, dockerHashes, ok, err := listServiceHashes(etcdCli)
        if err != nil {
            fmt.Println(err)
            stream.Reset()
            return
        }

        respInfo := common.ListResponse{contentHashes, dockerHashes, ok}
        respBytes, err := json.Marshal(respInfo)
        if err != nil {
            fmt.Println(err)
            stream.Reset()
            return
        }

        fmt.Println("List response: ", string(respBytes))

        err = p2putil.WriteMsg(stream, respBytes)
        if err != nil {
            fmt.Println(err)
            return
        }
    }
}

func handleAdd(etcdCli *clientv3.Client) func(network.Stream) {
    return func(stream network.Stream) {
        data, err := p2putil.ReadMsg(stream)
        if err != nil {
            fmt.Println(err)
            return
        }
        
        reqStr := strings.TrimSpace(string(data))
        fmt.Println("Add request:", reqStr)

        var reqInfo common.AddRequest
        err = json.Unmarshal([]byte(reqStr), &reqInfo)
        if err != nil {
            fmt.Println(err)
            stream.Reset()
            return
        }

        putData := etcdData{reqInfo.ContentHash, reqInfo.DockerHash}
        putDataBytes, err := json.Marshal(putData)
        if err != nil {
            fmt.Println(err)
            stream.Reset()
            return
        }

        ctx := context.Background()
        _, err = etcdCli.Put(ctx, reqInfo.ServiceName, string(putDataBytes))
        if err != nil {
            fmt.Println(err)
            stream.Reset()
            return
        }
        
        respStr := fmt.Sprintf("Added %s:%s",
            reqInfo.ServiceName, string(putDataBytes))
        
        fmt.Println("Add response: ", respStr)
        err = p2putil.WriteMsg(stream, []byte(respStr))
        if err != nil {
            fmt.Println(err)
            return
        }
    }
}

func lookupServiceHash(etcdCli *clientv3.Client, query string) (
    contentHash, dockerHash string, ok bool, err error) {
    
    contentHashes, dockerHashes, ok, err := etcdGet(etcdCli, query, false)
    if len(contentHashes) > 0 {
        contentHash = contentHashes[0]
    }
    if len(dockerHashes) > 0 {
        dockerHash = dockerHashes[0]
    }
    return contentHash, dockerHash, ok, err
}

func listServiceHashes(etcdCli *clientv3.Client) (
    contentHashes, dockerHashes []string, ok bool, err error) {
    
    return etcdGet(etcdCli, "", true)
}

func etcdGet(etcdCli *clientv3.Client, query string, withPrefix bool) (
    contentHashes, dockerHashes []string, queryOk bool, err error) {

    ctx := context.Background()
    var getResp *clientv3.GetResponse
    if withPrefix {
        getResp, err = etcdCli.Get(ctx, query, clientv3.WithPrefix())
    } else {
        getResp, err = etcdCli.Get(ctx, query)
    }
    if err != nil {
        return contentHashes, dockerHashes, queryOk, err
    }

    queryOk = len(getResp.Kvs) > 0
    for _, kv := range getResp.Kvs {
        var getData etcdData
        err = json.Unmarshal(kv.Value, &getData)
        if err != nil {
            return contentHashes, dockerHashes, queryOk, err
        }
        contentHashes = append(contentHashes, getData.ContentHash)
        dockerHashes = append(dockerHashes, getData.DockerHash)
    }

    return contentHashes, dockerHashes, queryOk, nil
}

type memberAddRequest struct {
    MemberName string
    MemberPeerUrl string
}

var memberAddProtocolID protocol.ID = "/memberadd/1.0"

func sendMemberAddRequest(
    newMemName, newMemPeerUrl string, local bool, bootstrap string) (
    initialCluster string, err error) {

    ctx := context.Background()
    nodeConfig := p2pnode.NewConfig()
    if local {
        nodeConfig.BootstrapPeers = []string{}
    } else if bootstrap != "" {
        nodeConfig.BootstrapPeers = []string{bootstrap}
    }
    node, err := p2pnode.NewNode(ctx, nodeConfig)
    if err != nil {
        return "", err
    }
    defer node.Host.Close()
    defer node.DHT.Close()

    reqInfo := memberAddRequest{newMemName, newMemPeerUrl}
    reqBytes, err := json.Marshal(reqInfo)
    if err != nil {
        return "", err
    }

    response, err := common.SendRequestWithHostRouting(
        ctx, node.Host, node.RoutingDiscovery, memberAddProtocolID, reqBytes)
    if err != nil {
        return "", err
    }

    initialCluster = strings.TrimSpace(string(response))
    
    return initialCluster, nil
}

func handleMemberAdd(etcdCli *clientv3.Client) func(network.Stream) {
    return func(stream network.Stream) {
        data, err := p2putil.ReadMsg(stream)
        if err != nil {
            fmt.Println(err)
            return
        }
        
        reqStr := strings.TrimSpace(string(data))
        fmt.Println("Member add request:", reqStr)

        var reqInfo memberAddRequest
        err = json.Unmarshal([]byte(reqStr), &reqInfo)
        if err != nil {
            fmt.Println(err)
            stream.Reset()
            return
        }

        initialCluster, err := addEtcdMember(
            etcdCli, reqInfo.MemberName, reqInfo.MemberPeerUrl)
        if err != nil {
            fmt.Println(err)
            stream.Reset()
            return
        }
        
        fmt.Println("Member add response: ", initialCluster)
        err = p2putil.WriteMsg(stream, []byte(initialCluster))
        if err != nil {
            fmt.Println(err)
            return
        }
    }
}

func addEtcdMember(
    etcdCli *clientv3.Client, newMemName, newMemPeerUrl string) (
    initialCluster string, err error) {

    ctx := context.Background()
    memAddResp, err := etcdCli.MemberAdd(ctx, []string{newMemPeerUrl})
    if err != nil {
        return "", nil
    }

    newMemId := memAddResp.Member.ID

    clusterPeerUrls := []string{}
    for _, mem := range memAddResp.Members {
        name := mem.Name
        if mem.ID == newMemId {
            name = newMemName
        }
        for _, peerUrl := range mem.PeerURLs {
            clusterPeerUrls = append(
                clusterPeerUrls, fmt.Sprintf("%s=%s", name, peerUrl))
        }
    }

    initialCluster = strings.Join(clusterPeerUrls, ",")

    return initialCluster, nil
}
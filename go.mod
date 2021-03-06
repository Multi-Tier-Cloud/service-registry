module github.com/PhysarumSM/service-registry

go 1.13

// Replace while etcd doesn't support newer version of grpc
replace google.golang.org/grpc => google.golang.org/grpc v1.26.0

require (
	github.com/PhysarumSM/common v0.10.0
	github.com/PhysarumSM/docker-driver v0.3.0
	github.com/PhysarumSM/service-manager v0.3.0
	github.com/libp2p/go-libp2p v0.9.2
	github.com/libp2p/go-libp2p-core v0.5.6
	github.com/libp2p/go-libp2p-discovery v0.4.0
	github.com/libp2p/go-libp2p-kad-dht v0.7.11
	github.com/multiformats/go-multiaddr v0.2.2
	github.com/prometheus/client_golang v1.6.0
	//go.etcd.io/etcd v0.5.0-alpha.5.0.20200212203316-09304a4d8263
	go.etcd.io/etcd v3.3.22+incompatible
	golang.org/x/crypto v0.0.0-20200510223506-06a226fb4e37
)

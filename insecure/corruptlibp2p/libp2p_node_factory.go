package corruptlibp2p

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	madns "github.com/multiformats/go-multiaddr-dns"
	"github.com/rs/zerolog"
	corrupt "github.com/yhassanzadeh13/go-libp2p-pubsub"

	fcrypto "github.com/onflow/flow-go/crypto"
	"github.com/onflow/flow-go/model/flow"
	"github.com/onflow/flow-go/module"
	"github.com/onflow/flow-go/network/p2p"
	"github.com/onflow/flow-go/network/p2p/p2pbuilder"
)

// NewCorruptLibP2PNodeFactory wrapper around the original DefaultLibP2PNodeFactory. Nodes returned from this factory func will be corrupted libp2p nodes.
func NewCorruptLibP2PNodeFactory(
	log zerolog.Logger,
	chainID flow.ChainID,
	address string,
	flowKey fcrypto.PrivateKey,
	sporkId flow.Identifier,
	idProvider module.IdentityProvider,
	metrics module.LibP2PMetrics,
	resolver madns.BasicResolver,
	role string,
	connGaterCfg *p2pbuilder.ConnectionGaterConfig,
	peerManagerCfg *p2pbuilder.PeerManagerConfig,
	uniCfg *p2pbuilder.UnicastConfig,
	gossipSubCfg *p2pbuilder.GossipSubConfig,
	topicValidatorDisabled,
	withMessageSigning,
	withStrictSignatureVerification bool,
) p2p.LibP2PFactoryFunc {
	return func() (p2p.LibP2PNode, error) {
		if chainID != flow.BftTestnet {
			panic("illegal chain id for using corrupt libp2p node")
		}

		builder, err := p2pbuilder.DefaultNodeBuilder(
			log,
			address,
			flowKey,
			sporkId,
			idProvider,
			metrics,
			resolver,
			role,
			connGaterCfg,
			peerManagerCfg,
			gossipSubCfg,
			p2pbuilder.DefaultResourceManagerConfig(),
			uniCfg)

		if err != nil {
			return nil, fmt.Errorf("could not create corrupt libp2p node builder: %w", err)
		}
		if topicValidatorDisabled {
			builder.SetCreateNode(NewCorruptLibP2PNode)
		}

		overrideWithCorruptGossipSub(builder, WithMessageSigning(withMessageSigning), WithStrictSignatureVerification(withStrictSignatureVerification))
		return builder.Build()
	}
}

// CorruptGossipSubFactory returns a factory function that creates a new instance of the forked gossipsub module from
// github.com/yhassanzadeh13/go-libp2p-pubsub for the purpose of BFT testing and attack vector implementation.
func CorruptGossipSubFactory(routerOpts ...func(*corrupt.GossipSubRouter)) p2p.GossipSubFactoryFunc {
	factory := func(ctx context.Context, logger zerolog.Logger, host host.Host, cfg p2p.PubSubAdapterConfig) (p2p.PubSubAdapter, error) {
		adapter, router, err := NewCorruptGossipSubAdapter(ctx, logger, host, cfg)
		for _, opt := range routerOpts {
			opt(router)
		}
		return adapter, err
	}
	return factory
}

// CorruptGossipSubConfigFactory returns a factory function that creates a new instance of the forked gossipsub config
// from github.com/yhassanzadeh13/go-libp2p-pubsub for the purpose of BFT testing and attack vector implementation.
func CorruptGossipSubConfigFactory(opts ...CorruptPubSubAdapterConfigOption) p2p.GossipSubAdapterConfigFunc {
	return func(base *p2p.BasePubSubAdapterConfig) p2p.PubSubAdapterConfig {
		return NewCorruptPubSubAdapterConfig(base, opts...)
	}
}

// CorruptGossipSubConfigFactoryWithInspector returns a factory function that creates a new instance of the forked gossipsub config
// from github.com/yhassanzadeh13/go-libp2p-pubsub for the purpose of BFT testing and attack vector implementation.
func CorruptGossipSubConfigFactoryWithInspector(inspector func(peer.ID, *corrupt.RPC) error) p2p.GossipSubAdapterConfigFunc {
	return func(base *p2p.BasePubSubAdapterConfig) p2p.PubSubAdapterConfig {
		return NewCorruptPubSubAdapterConfig(base, WithInspector(inspector))
	}
}

func overrideWithCorruptGossipSub(builder p2p.NodeBuilder, opts ...CorruptPubSubAdapterConfigOption) {
	factory := CorruptGossipSubFactory()
	builder.SetGossipSubFactory(factory, CorruptGossipSubConfigFactory(opts...))
}

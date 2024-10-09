package tracing

import (
	"github.com/cometbft/cometbft/types"
	"github.com/ethereum/go-ethereum/core/tracing"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"math/big"
)

type (
	OnCosmosBlockStart func(CosmosStartBlockEvent)
	OnCosmosBlockEnd   func(CosmosEndBlockEvent, error)
)

type Hooks struct {
	*tracing.Hooks

	OnCosmosBlockStart OnCosmosBlockStart
	OnCosmosBlockEnd   OnCosmosBlockEnd
}

type CosmosStartBlockEvent struct {
	CosmosHeader types.Header
	BaseFee      *big.Int
	GasLimit     uint64
	Coinbase     []byte
	Finalized    *ethtypes.Header
}

type CosmosEndBlockEvent struct {
	LogsBloom []byte
}

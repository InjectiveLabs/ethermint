package tracers

import (
	"context"
	sdk "github.com/cosmos/cosmos-sdk/types"
	cosmostracing "github.com/ethereum/go-ethereum/core/tracing"
	"github.com/evmos/ethermint/x/evm/tracing"
)

// BlockchainTracerFactory is a function that creates a new [BlockchainTracer].
// It's going to receive the parsed URL from the `live-evm-tracer` flag.
//
// The scheme of the URL is going to be used to determine which tracer to use
// by the registry.
type BlockchainTracerFactory = func(backwardCompatibility bool) (*tracing.Hooks, error)

type CtxBlockchainTracerKeyType string

const CtxBlockchainTracerKey = CtxBlockchainTracerKeyType("evm_and_state_logger")

func SetCtxBlockchainTracer(ctx context.Context, hooks *cosmostracing.Hooks) context.Context {
	return context.WithValue(ctx, CtxBlockchainTracerKey, hooks)
}

func GetCtxBlockchainTracer(ctx sdk.Context) *cosmostracing.Hooks {
	rawVal := ctx.Context().Value(CtxBlockchainTracerKey)
	if rawVal == nil {
		return nil
	}
	logger, ok := rawVal.(*cosmostracing.Hooks)
	if !ok {
		return nil
	}
	return logger
}

func GetCtxEthTracingHooks(ctx sdk.Context) *cosmostracing.Hooks {
	if logger := GetCtxBlockchainTracer(ctx); logger != nil {
		return logger
	}

	return nil
}

type TxTracerHooks struct {
	Hooks *cosmostracing.Hooks

	OnTxReset  func()
	OnTxCommit func()
}

func (h TxTracerHooks) InjectInContext(ctx sdk.Context) context.Context {
	return SetCtxBlockchainTracer(ctx, h.Hooks)
}

func (h TxTracerHooks) Reset() {
	if h.OnTxReset != nil {
		h.OnTxReset()
	}
}

func (h TxTracerHooks) Commit() {
	if h.OnTxCommit != nil {
		h.OnTxCommit()
	}
}

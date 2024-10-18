package tracing

import (
	"context"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type CtxBlockchainTracerKeyType string

const CtxBlockchainTracerKey = CtxBlockchainTracerKeyType("evm_and_state_logger")

func SetCtxBlockchainTracer(ctx sdk.Context, logger *Hooks) sdk.Context {
	return ctx.WithContext(context.WithValue(ctx.Context(), CtxBlockchainTracerKey, logger))
}

// GetCtxBlockchainTracer function to get the Cosmos specific [tracing.Hooks] struct
// used to trace EVM blocks and transactions.
func GetCtxBlockchainTracer(ctx sdk.Context) *Hooks {
	rawVal := ctx.Context().Value(CtxBlockchainTracerKey)
	if rawVal == nil {
		return nil
	}
	logger, ok := rawVal.(*Hooks)
	if !ok {
		return nil
	}
	return logger
}

func GetCtxEthTracingHooks(ctx sdk.Context) *Hooks {
	if logger := GetCtxBlockchainTracer(ctx); logger != nil {
		return logger
	}

	return nil
}

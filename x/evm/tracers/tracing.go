package tracers

import (
	"github.com/evmos/ethermint/x/evm/tracing"
)

// BlockchainTracerFactory is a function that creates a new [BlockchainTracer].
// It's going to receive the parsed URL from the `live-evm-tracer` flag.
//
// The scheme of the URL is going to be used to determine which tracer to use
// by the registry.
type BlockchainTracerFactory = func(backwardCompatibility bool) (*tracing.Hooks, error)

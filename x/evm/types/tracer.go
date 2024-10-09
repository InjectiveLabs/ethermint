// Copyright 2021 Evmos Foundation
// This file is part of Evmos' Ethermint library.
//
// The Ethermint library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Ethermint library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Ethermint library. If not, see https://github.com/evmos/ethermint/blob/main/LICENSE
package types

import (
	"fmt"
	cosmostracing "github.com/evmos/ethermint/x/evm/tracing"
	"os"

	"github.com/ethereum/go-ethereum/eth/tracers"
	cosmostracers "github.com/evmos/ethermint/x/evm/tracers"

	_ "github.com/ethereum/go-ethereum/eth/tracers/live"
	_ "github.com/evmos/ethermint/x/evm/tracers"

	"github.com/ethereum/go-ethereum/eth/tracers/logger"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
)

const (
	TracerAccessList = "access_list"
	TracerJSON       = "json"
	TracerStruct     = "struct"
	TracerMarkdown   = "markdown"
	Firehose         = "firehose"
)

// NewTracer creates a new Logger tracer to collect execution traces from an
// EVM transaction.
func NewTracer(tracer string, msg *core.Message, rules params.Rules) *tracers.Tracer {
	// TODO: enable additional log configuration
	logCfg := &logger.Config{
		Debug: true,
	}

	var hooks *tracing.Hooks

	switch tracer {
	case TracerAccessList:
		preCompiles := vm.DefaultActivePrecompiles(rules)
		hooks = logger.NewAccessListTracer(msg.AccessList, msg.From, *msg.To, preCompiles).Hooks()
	case TracerJSON:
		hooks = logger.NewJSONLogger(logCfg, os.Stderr)
	case TracerMarkdown:
		hooks = logger.NewMarkdownLogger(logCfg, os.Stdout).Hooks() // TODO: Stderr ?
	case TracerStruct:
		hooks = logger.NewStructLogger(logCfg).Hooks()
	case Firehose:
		hooks, _ = tracers.LiveDirectory.New("firehose", nil)
	default:
		// Use noop tracer by default
		hooks, _ = tracers.LiveDirectory.New("noop", nil)
	}

	return &tracers.Tracer{
		Hooks: hooks,
	}
}

func NewLiveTracer(tracer string) (*tracers.Tracer, error) {
	h, err := tracers.LiveDirectory.New(tracer, nil)
	if err != nil {
		return nil, fmt.Errorf("initializing live tracer %s: %w", tracer, err)
	}

	return &tracers.Tracer{
		Hooks: h,
	}, nil
}

func NewFirehoseCosmosLiveTracer() (*cosmostracing.Hooks, error) {
	h, err := cosmostracers.NewCosmosFirehoseTracer(false)
	if err != nil {
		return nil, fmt.Errorf("initializing live tracer firehose: %w", err)
	}

	return h, nil
}

// TxTraceResult is the result of a single transaction trace during a block trace.
type TxTraceResult struct {
	Result interface{} `json:"result,omitempty"` // Trace results produced by the tracer
	Error  string      `json:"error,omitempty"`  // Trace failure produced by the tracer
}

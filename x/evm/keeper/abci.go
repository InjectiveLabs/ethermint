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
package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"math/big"
)

// BeginBlock sets the sdk Context and EIP155 chain id to the Keeper.
func (k *Keeper) BeginBlock(ctx sdk.Context) error {
	k.WithChainID(ctx)

	// cache parameters that's common for the whole block.
	if _, err := k.EVMBlockConfig(ctx, k.ChainID()); err != nil {
		return err
	}

	if k.evmTracer != nil && k.evmTracer.OnBlockStart != nil {
		b := types.NewBlock(&types.Header{
			Number:     big.NewInt(ctx.BlockHeight()),
			Time:       uint64(ctx.BlockTime().Unix()),
			ParentHash: ethcommon.BytesToHash(ctx.BlockHeader().LastBlockId.Hash),
			Coinbase:   ethcommon.BytesToAddress(ctx.BlockHeader().ProposerAddress),
		}, nil, nil, nil)

		finalizedHeaderNumber := ctx.BlockHeight() - 1
		if ctx.BlockHeight() == 0 {
			finalizedHeaderNumber = 0
		}

		finalizedHeader := &types.Header{
			Number: big.NewInt(finalizedHeaderNumber),
		}

		k.evmTracer.OnBlockStart(tracing.BlockEvent{
			Block:     b,
			TD:        big.NewInt(0),
			Finalized: finalizedHeader,
		})
	}

	return nil
}

// EndBlock also retrieves the bloom filter value from the transient store and commits it to the
// KVStore. The EVM end block logic doesn't update the validator set, thus it returns
// an empty slice.
func (k *Keeper) EndBlock(ctx sdk.Context) error {
	k.CollectTxBloom(ctx)
	k.RemoveParamsCache(ctx)

	if k.evmTracer != nil && k.evmTracer.OnBlockEnd != nil {
		defer func() {
			k.evmTracer.OnBlockEnd(nil)
		}()
	}

	return nil
}

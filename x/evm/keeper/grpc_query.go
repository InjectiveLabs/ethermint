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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	cosmostracing "github.com/evmos/ethermint/x/evm/tracing"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/eth/tracers"
	"github.com/ethereum/go-ethereum/eth/tracers/logger"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	ethparams "github.com/ethereum/go-ethereum/params"

	rpctypes "github.com/evmos/ethermint/rpc/types"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm/types"
)

var _ types.QueryServer = Keeper{}

const (
	defaultTraceTimeout = 5 * time.Second
)

// Account implements the Query/Account gRPC method
func (k Keeper) Account(c context.Context, req *types.QueryAccountRequest) (*types.QueryAccountResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if err := ethermint.ValidateAddress(req.Address); err != nil {
		return nil, status.Error(
			codes.InvalidArgument, err.Error(),
		)
	}

	addr := common.HexToAddress(req.Address)

	ctx := sdk.UnwrapSDKContext(c)
	acct := k.GetAccountOrEmpty(ctx, addr)
	balance := k.GetEVMDenomBalance(ctx, addr)

	return &types.QueryAccountResponse{
		Balance:  balance.String(),
		CodeHash: common.BytesToHash(acct.CodeHash).Hex(),
		Nonce:    acct.Nonce,
	}, nil
}

func (k Keeper) CosmosAccount(c context.Context, req *types.QueryCosmosAccountRequest) (*types.QueryCosmosAccountResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if err := ethermint.ValidateAddress(req.Address); err != nil {
		return nil, status.Error(
			codes.InvalidArgument, err.Error(),
		)
	}

	ctx := sdk.UnwrapSDKContext(c)

	ethAddr := common.HexToAddress(req.Address)
	cosmosAddr := sdk.AccAddress(ethAddr.Bytes())

	account := k.accountKeeper.GetAccount(ctx, cosmosAddr)
	res := types.QueryCosmosAccountResponse{
		CosmosAddress: cosmosAddr.String(),
	}

	if account != nil {
		res.Sequence = account.GetSequence()
		res.AccountNumber = account.GetAccountNumber()
	}

	return &res, nil
}

// ValidatorAccount implements the Query/Balance gRPC method
func (k Keeper) ValidatorAccount(c context.Context, req *types.QueryValidatorAccountRequest) (*types.QueryValidatorAccountResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	consAddr, err := sdk.ConsAddressFromBech32(req.ConsAddress)
	if err != nil {
		return nil, status.Error(
			codes.InvalidArgument, err.Error(),
		)
	}

	ctx := sdk.UnwrapSDKContext(c)

	validator, err := k.stakingKeeper.GetValidatorByConsAddr(ctx, consAddr)
	if err != nil {
		return nil, fmt.Errorf("validator not found for %s, %w", consAddr.String(), err)
	}

	bz, err := sdk.ValAddressFromBech32(validator.GetOperator())
	if err != nil {
		return nil, err
	}
	accAddr := sdk.AccAddress(bz)
	res := types.QueryValidatorAccountResponse{
		AccountAddress: accAddr.String(),
	}

	account := k.accountKeeper.GetAccount(ctx, accAddr)
	if account != nil {
		res.Sequence = account.GetSequence()
		res.AccountNumber = account.GetAccountNumber()
	}

	return &res, nil
}

// Balance implements the Query/Balance gRPC method
func (k Keeper) Balance(c context.Context, req *types.QueryBalanceRequest) (*types.QueryBalanceResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if err := ethermint.ValidateAddress(req.Address); err != nil {
		return nil, status.Error(
			codes.InvalidArgument,
			types.ErrZeroAddress.Error(),
		)
	}

	ctx := sdk.UnwrapSDKContext(c)

	balanceInt := k.GetEVMDenomBalance(ctx, common.HexToAddress(req.Address))

	return &types.QueryBalanceResponse{
		Balance: balanceInt.String(),
	}, nil
}

// Storage implements the Query/Storage gRPC method
func (k Keeper) Storage(c context.Context, req *types.QueryStorageRequest) (*types.QueryStorageResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if err := ethermint.ValidateAddress(req.Address); err != nil {
		return nil, status.Error(
			codes.InvalidArgument,
			types.ErrZeroAddress.Error(),
		)
	}

	ctx := sdk.UnwrapSDKContext(c)

	address := common.HexToAddress(req.Address)
	key := common.HexToHash(req.Key)

	state := k.GetState(ctx, address, key)
	stateHex := state.Hex()

	return &types.QueryStorageResponse{
		Value: stateHex,
	}, nil
}

// Code implements the Query/Code gRPC method
func (k Keeper) Code(c context.Context, req *types.QueryCodeRequest) (*types.QueryCodeResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if err := ethermint.ValidateAddress(req.Address); err != nil {
		return nil, status.Error(
			codes.InvalidArgument,
			types.ErrZeroAddress.Error(),
		)
	}

	ctx := sdk.UnwrapSDKContext(c)

	address := common.HexToAddress(req.Address)
	acct := k.GetAccount(ctx, address)

	var code []byte
	if acct != nil && acct.IsContract() {
		code = k.GetCode(ctx, common.BytesToHash(acct.CodeHash))
	}

	return &types.QueryCodeResponse{
		Code: code,
	}, nil
}

// Params implements the Query/Params gRPC method
func (k Keeper) Params(c context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	params := k.GetParams(ctx)

	return &types.QueryParamsResponse{
		Params: params,
	}, nil
}

// EthCall implements eth_call rpc api.
func (k Keeper) EthCall(c context.Context, req *types.EthCallRequest) (*types.MsgEthereumTxResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	ctx := sdk.UnwrapSDKContext(c)
	ctx = ctx.WithProposer(GetProposerAddress(ctx, req.ProposerAddress))

	var args types.TransactionArgs
	err := json.Unmarshal(req.Args, &args)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	chainID, err := getChainID(ctx, req.ChainId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	cfg, err := k.EVMConfig(ctx, chainID, common.Hash{})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	var overrides rpctypes.StateOverride
	if len(req.Overrides) > 0 {
		if err := json.Unmarshal(req.Overrides, &overrides); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		cfg.Overrides = &overrides
	}

	// ApplyMessageWithConfig expect correct nonce set in msg
	nonce := k.GetNonce(ctx, args.GetFrom())
	args.Nonce = (*hexutil.Uint64)(&nonce)

	msg, err := args.ToMessage(req.GasCap, cfg.BaseFee)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// pass false to not commit StateDB
	res, err := k.ApplyMessageWithConfig(ctx, msg, cfg, false)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return res, nil
}

// EstimateGas implements eth_estimateGas rpc api.
func (k Keeper) EstimateGas(c context.Context, req *types.EthCallRequest) (*types.EstimateGasResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	ctx := sdk.UnwrapSDKContext(c)
	ctx = ctx.WithProposer(GetProposerAddress(ctx, req.ProposerAddress))
	chainID, err := getChainID(ctx, req.ChainId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if req.GasCap < ethparams.TxGas {
		return nil, status.Error(codes.InvalidArgument, "gas cap cannot be lower than 21,000")
	}

	var args types.TransactionArgs
	err = json.Unmarshal(req.Args, &args)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// Binary search the gas requirement, as it may be higher than the amount used
	var (
		lo     = ethparams.TxGas - 1
		hi     uint64
		gasCap uint64
	)

	// Determine the highest gas limit can be used during the estimation.
	if args.Gas != nil && uint64(*args.Gas) >= ethparams.TxGas {
		hi = uint64(*args.Gas)
	} else {
		// Query block gas limit
		params := ctx.ConsensusParams()
		if params.Block != nil && params.Block.MaxGas > 0 {
			hi = uint64(params.Block.MaxGas)
		} else {
			hi = req.GasCap
		}
	}

	// TODO: Recap the highest gas limit with account's available balance.

	// Recap the highest gas allowance with specified gascap.
	if req.GasCap != 0 && hi > req.GasCap {
		hi = req.GasCap
	}
	gasCap = hi
	cfg, err := k.EVMConfig(ctx, chainID, common.Hash{})
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load evm config")
	}

	// ApplyMessageWithConfig expect correct nonce set in msg
	nonce := k.GetNonce(ctx, args.GetFrom())
	args.Nonce = (*hexutil.Uint64)(&nonce)

	// convert the tx args to an ethereum message
	msg, err := args.ToMessage(req.GasCap, cfg.BaseFee)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// NOTE: the errors from the executable below should be consistent with go-ethereum,
	// so we don't wrap them with the gRPC status code

	// Create a helper to check if a gas allowance results in an executable transaction
	executable := func(gas uint64) (vmError bool, rsp *types.MsgEthereumTxResponse, err error) {
		// update the message with the new gas value
		msg = &core.Message{
			From:              msg.From,
			To:                msg.To,
			Nonce:             msg.Nonce,
			Value:             msg.Value,
			GasLimit:          gas,
			GasPrice:          msg.GasPrice,
			GasFeeCap:         msg.GasFeeCap,
			GasTipCap:         msg.GasTipCap,
			Data:              msg.Data,
			AccessList:        msg.AccessList,
			SkipAccountChecks: msg.SkipAccountChecks,
		}

		// pass false to not commit StateDB
		rsp, err = k.ApplyMessageWithConfig(ctx, msg, cfg, false)
		if err != nil {
			if errors.Is(err, core.ErrIntrinsicGas) {
				return true, nil, nil // Special case, raise gas limit
			}
			return true, nil, err // Bail out
		}
		return len(rsp.VmError) > 0, rsp, nil
	}

	// Execute the binary search and hone in on an executable gas limit
	hi, err = types.BinSearch(lo, hi, executable)
	if err != nil {
		return nil, err
	}

	// Reject the transaction as invalid if it still fails at the highest allowance
	if hi == gasCap {
		failed, result, err := executable(hi)
		if err != nil {
			return nil, err
		}

		if failed {
			if result != nil && result.VmError != vm.ErrOutOfGas.Error() {
				if result.VmError == vm.ErrExecutionReverted.Error() {
					return &types.EstimateGasResponse{
						Ret:     result.Ret,
						VmError: result.VmError,
					}, nil
				}
				return nil, errors.New(result.VmError)
			}
			// Otherwise, the specified gas cap is too low
			return nil, fmt.Errorf("gas required exceeds allowance (%d)", gasCap)
		}
	}
	return &types.EstimateGasResponse{Gas: hi}, nil
}

type traceRequest interface {
	comparable
	GetTraceConfig() *types.TraceConfig
	GetBlockNumber() int64
	GetBlockTime() time.Time
	GetBlockHash() string
	GetChainId() int64
	GetProposerAddress() sdk.ConsAddress
}

func execTrace[T traceRequest](
	c context.Context,
	req T,
	k Keeper,
	msgCb func(
		ctx sdk.Context,
		cfg *EVMConfig,
	) (*core.Message, error),
) ([]byte, error) {
	var zero T
	if req == zero {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if traceConfig := req.GetTraceConfig(); traceConfig != nil && traceConfig.Limit < 0 {
		return nil, status.Errorf(codes.InvalidArgument, "output limit cannot be negative, got %d", traceConfig.Limit)
	}

	// get the context of block beginning
	contextHeight := req.GetBlockNumber()
	if contextHeight < 1 {
		// 0 is a special value in `ContextWithHeight`
		contextHeight = 1
	}

	ctx := sdk.UnwrapSDKContext(c)
	ctx = ctx.WithBlockHeight(contextHeight)
	ctx = ctx.WithBlockTime(req.GetBlockTime())
	ctx = ctx.WithHeaderHash(common.Hex2Bytes(req.GetBlockHash()))
	ctx = ctx.WithProposer(GetProposerAddress(ctx, req.GetProposerAddress()))

	chainID, err := getChainID(ctx, req.GetChainId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	cfg, err := k.EVMConfig(ctx, chainID, common.Hash{})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to load evm config: %s", err.Error())
	}

	msg, err := msgCb(ctx, cfg)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	result, _, err := k.prepareTrace(ctx, cfg, msg, req.GetTraceConfig(), false)
	if err != nil {
		// error will be returned with detail status from traceTx
		return nil, err
	}

	resultData, err := json.Marshal(result)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return resultData, nil
}

// TraceTx configures a new tracer according to the provided configuration, and
// executes the given message in the provided environment. The return value will
// be tracer dependent.
func (k Keeper) TraceTx(c context.Context, req *types.QueryTraceTxRequest) (*types.QueryTraceTxResponse, error) {
	resultData, err := execTrace(
		c,
		req,
		k,
		func(ctx sdk.Context, cfg *EVMConfig) (*core.Message, error) {
			signer := ethtypes.MakeSigner(cfg.ChainConfig, big.NewInt(ctx.BlockHeight()), cfg.BlockTime)

			for i, tx := range req.Predecessors {
				ethTx := tx.AsTransaction()
				msg, err := core.TransactionToMessage(ethTx, signer, cfg.BaseFee)
				if err != nil {
					continue
				}
				cfg.TxConfig.TxHash = ethTx.Hash()
				cfg.TxConfig.TxIndex = uint(i)
				rsp, err := k.ApplyMessageWithConfig(ctx, msg, cfg, true)
				if err != nil {
					continue
				}
				cfg.TxConfig.LogIndex += uint(len(rsp.Logs))
			}

			tx := req.Msg.AsTransaction()
			cfg.TxConfig.TxHash = tx.Hash()
			if len(req.Predecessors) > 0 {
				cfg.TxConfig.TxIndex++
			}

			return core.TransactionToMessage(tx, signer, cfg.BaseFee)
		},
	)
	if err != nil {
		return nil, err
	}

	return &types.QueryTraceTxResponse{
		Data: resultData,
	}, nil
}

// TraceBlock configures a new tracer according to the provided configuration, and
// executes the given message in the provided environment for all the transactions in the queried block.
// The return value will be tracer dependent.
func (k Keeper) TraceBlock(c context.Context, req *types.QueryTraceBlockRequest) (*types.QueryTraceBlockResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	if req.TraceConfig != nil && req.TraceConfig.Limit < 0 {
		return nil, status.Errorf(codes.InvalidArgument, "output limit cannot be negative, got %d", req.TraceConfig.Limit)
	}

	// get the context of block beginning
	contextHeight := req.BlockNumber
	if contextHeight < 1 {
		// 0 is a special value in `ContextWithHeight`
		contextHeight = 1
	}

	ctx := sdk.UnwrapSDKContext(c)
	ctx = ctx.WithBlockHeight(contextHeight)
	ctx = ctx.WithBlockTime(req.BlockTime)
	ctx = ctx.WithHeaderHash(common.Hex2Bytes(req.BlockHash))
	ctx = ctx.WithProposer(GetProposerAddress(ctx, req.ProposerAddress))
	chainID, err := getChainID(ctx, req.ChainId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	cfg, err := k.EVMConfig(ctx, chainID, common.Hash{})
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load evm config")
	}
	signer := ethtypes.MakeSigner(cfg.ChainConfig, big.NewInt(ctx.BlockHeight()), cfg.BlockTime)
	txsLength := len(req.Txs)
	results := make([]*types.TxTraceResult, 0, txsLength)

	for i, tx := range req.Txs {
		result := types.TxTraceResult{}
		ethTx := tx.AsTransaction()
		cfg.TxConfig.TxHash = ethTx.Hash()
		cfg.TxConfig.TxIndex = uint(i)
		msg, err := core.TransactionToMessage(ethTx, signer, cfg.BaseFee)
		if err != nil {
			result.Error = status.Error(codes.Internal, err.Error()).Error()
		} else {
			traceResult, logIndex, err := k.prepareTrace(ctx, cfg, msg, req.TraceConfig, true)
			if err != nil {
				result.Error = err.Error()
			} else {
				cfg.TxConfig.LogIndex = logIndex
				result.Result = traceResult
			}
		}
		results = append(results, &result)
	}

	resultData, err := json.Marshal(results)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryTraceBlockResponse{
		Data: resultData,
	}, nil
}

// TraceCall configures a new tracer according to the provided configuration, and
// executes the given call in the provided environment. The return value will
// be tracer dependent.
func (k Keeper) TraceCall(c context.Context, req *types.QueryTraceCallRequest) (*types.QueryTraceCallResponse, error) {
	resultData, err := execTrace(
		c,
		req,
		k,
		func(ctx sdk.Context, cfg *EVMConfig) (*core.Message, error) {
			var args types.TransactionArgs
			err := json.Unmarshal(req.Args, &args)
			if err != nil {
				return nil, err
			}

			// ApplyMessageWithConfig expect correct nonce set in msg
			nonce := k.GetNonce(ctx, args.GetFrom())
			args.Nonce = (*hexutil.Uint64)(&nonce)

			msg, err := args.ToMessage(req.GasCap, cfg.BaseFee)
			if err != nil {
				return nil, err
			}
			return msg, nil
		},
	)
	if err != nil {
		return nil, err
	}

	return &types.QueryTraceCallResponse{
		Data: resultData,
	}, nil
}

// prepareTrace prepare trace on one Ethereum message, it returns a tuple: (traceResult, nextLogIndex, error).
func (k *Keeper) prepareTrace(
	ctx sdk.Context,
	cfg *EVMConfig,
	msg *core.Message,
	traceConfig *types.TraceConfig,
	commitMessage bool,
) (*interface{}, uint, error) {
	txConfig := cfg.TxConfig
	// Assemble the structured logger or the JavaScript tracer
	var (
		tracer    *tracers.Tracer
		overrides *ethparams.ChainConfig
		err       error
		timeout   = defaultTraceTimeout
	)

	if traceConfig == nil {
		traceConfig = &types.TraceConfig{}
	}

	if traceConfig.Overrides != nil {
		overrides = traceConfig.Overrides.EthereumConfig(cfg.ChainConfig.ChainID)
	}

	logConfig := logger.Config{
		EnableMemory:     traceConfig.EnableMemory,
		DisableStorage:   traceConfig.DisableStorage,
		DisableStack:     traceConfig.DisableStack,
		EnableReturnData: traceConfig.EnableReturnData,
		Debug:            traceConfig.Debug,
		Limit:            int(traceConfig.Limit),
		Overrides:        overrides,
	}

	logger := logger.NewStructLogger(&logConfig)
	tracer = &tracers.Tracer{
		Hooks:     logger.Hooks(),
		GetResult: logger.GetResult,
		Stop:      logger.Stop,
	}

	txIndex, err := ethermint.SafeInt(txConfig.TxIndex)
	if err != nil {
		return nil, 0, status.Error(codes.Internal, err.Error())
	}

	tCtx := &tracers.Context{
		BlockHash: txConfig.BlockHash,
		TxIndex:   txIndex,
		TxHash:    txConfig.TxHash,
	}
	if traceConfig.Tracer != "" {
		var cfg json.RawMessage
		if traceConfig.TracerJsonConfig != "" {
			cfg = json.RawMessage(traceConfig.TracerJsonConfig)
		}
		t, err := tracers.DefaultDirectory.New(traceConfig.Tracer, tCtx, cfg)
		if err != nil {
			return nil, 0, status.Error(codes.Internal, err.Error())
		}
		tracer = t
	}

	// Define a meaningful timeout of a single transaction trace
	if traceConfig.Timeout != "" {
		if timeout, err = time.ParseDuration(traceConfig.Timeout); err != nil {
			return nil, 0, status.Errorf(codes.InvalidArgument, "timeout value: %s", err.Error())
		}
	}

	// Handle timeouts and RPC cancellations
	deadlineCtx, cancel := context.WithTimeout(ctx.Context(), timeout)
	defer cancel()

	go func() {
		<-deadlineCtx.Done()
		if errors.Is(deadlineCtx.Err(), context.DeadlineExceeded) {
			tracer.Stop(errors.New("execution timeout"))
		}
	}()

	if traceConfig.StateOverrides != nil {
		var stateOverrides rpctypes.StateOverride
		if err := json.Unmarshal(traceConfig.StateOverrides, &stateOverrides); err != nil {
			return nil, 0, status.Error(codes.InvalidArgument, err.Error())
		}

		cfg.Overrides = &stateOverrides
	}

	if traceConfig.BlockOverrides != nil {
		var blockOverrides rpctypes.BlockOverrides
		if err := json.Unmarshal(traceConfig.BlockOverrides, &blockOverrides); err != nil {
			return nil, 0, status.Error(codes.InvalidArgument, err.Error())
		}

		cfg.BlockOverrides = &blockOverrides
	}

	cfg.Tracer = &cosmostracing.Hooks{
		Hooks: tracer.Hooks,
	}
	cfg.DebugTrace = true
	res, err := k.ApplyMessageWithConfig(ctx, msg, cfg, commitMessage)
	if err != nil {
		return nil, 0, status.Error(codes.Internal, err.Error())
	}

	if res.VmError != "" {
		if res.VmError == vm.ErrInsufficientBalance.Error() {
			return nil, 0, status.Error(codes.Internal, res.VmError)
		}
	}

	var result interface{}
	result, err = tracer.GetResult()
	if err != nil {
		return nil, 0, status.Error(codes.Internal, err.Error())
	}

	return &result, txConfig.LogIndex + uint(len(res.Logs)), nil
}

// BaseFee implements the Query/BaseFee gRPC method
func (k Keeper) BaseFee(c context.Context, _ *types.QueryBaseFeeRequest) (*types.QueryBaseFeeResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)

	params := k.GetParams(ctx)
	ethCfg := params.ChainConfig.EthereumConfig(k.eip155ChainID)
	baseFee := k.GetBaseFee(ctx, ethCfg)

	res := &types.QueryBaseFeeResponse{}
	if baseFee != nil {
		aux := sdkmath.NewIntFromBigInt(baseFee)
		res.BaseFee = &aux
	}

	return res, nil
}

// getChainID parse chainID from current context if not provided
func getChainID(ctx sdk.Context, chainID int64) (*big.Int, error) {
	if chainID == 0 {
		return ethermint.ParseChainID(ctx.ChainID())
	}
	return big.NewInt(chainID), nil
}

package testutil

import (
	"encoding/json"
	"math/big"
	"time"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/evmos/ethermint/app"
	"github.com/evmos/ethermint/crypto/ethsecp256k1"
	"github.com/evmos/ethermint/server/config"
	"github.com/evmos/ethermint/tests"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/types"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type EVMTestSuite struct {
	suite.Suite

	Ctx sdk.Context
	App *app.EthermintApp
}

func (suite *EVMTestSuite) SetupTest() {
	suite.SetupTestWithCb(nil)
}

func (suite *EVMTestSuite) SetupTestWithCb(patch func(*app.EthermintApp, app.GenesisState) app.GenesisState) {
	checkTx := false
	suite.App = app.Setup(checkTx, patch)
	suite.Ctx = suite.App.BaseApp.NewContext(checkTx, tmproto.Header{
		Height:  1,
		ChainID: app.ChainID,
		Time:    time.Now().UTC(),
	})
}

func (suite *EVMTestSuite) StateDB() *statedb.StateDB {
	return statedb.New(suite.Ctx, suite.App.EvmKeeper, statedb.NewEmptyTxConfig(common.BytesToHash(suite.Ctx.HeaderHash().Bytes())))
}

type EVMTestSuiteWithAccount struct {
	EVMTestSuite
	Address common.Address
	Signer  keyring.Signer
}

func (suite *EVMTestSuiteWithAccount) SetupTest() {
	suite.EVMTestSuite.SetupTest()
	suite.SetupAccount()
}

func (suite *EVMTestSuiteWithAccount) SetupTestWithCb(patch func(*app.EthermintApp, app.GenesisState) app.GenesisState) {
	suite.EVMTestSuite.SetupTestWithCb(patch)
	suite.SetupAccount()
}

func (suite *EVMTestSuiteWithAccount) SetupAccountWithT(t require.TestingT) {
	// account key, use a constant account to keep unit test deterministic.
	ecdsaPriv, err := crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	require.NoError(t, err)
	priv := &ethsecp256k1.PrivKey{
		Key: crypto.FromECDSA(ecdsaPriv),
	}
	suite.Address = common.BytesToAddress(priv.PubKey().Address().Bytes())
	suite.Signer = tests.NewSigner(priv)
}

func (suite *EVMTestSuiteWithAccount) SetupAccount() {
	suite.SetupAccountWithT(suite.T())
}

// DeployTestContract deploy a test erc20 contract and returns the contract address
func (suite *EVMTestSuiteWithAccount) DeployTestContractWithT(
	owner common.Address,
	supply *big.Int,
	enableFeemarket bool,
	queryClient types.QueryClient,
	signer keyring.Signer,
	t require.TestingT,
) common.Address {
	ctx := sdk.WrapSDKContext(suite.Ctx)
	chainID := suite.App.EvmKeeper.ChainID()
	ctorArgs, err := types.ERC20Contract.ABI.Pack("", owner, supply)
	require.NoError(t, err)
	nonce := suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address)
	data := append(types.ERC20Contract.Bin, ctorArgs...) //nolint: gocritic
	args, err := json.Marshal(&types.TransactionArgs{
		From: &suite.Address,
		Data: (*hexutil.Bytes)(&data),
	})
	require.NoError(t, err)
	res, err := queryClient.EstimateGas(ctx, &types.EthCallRequest{
		Args:            args,
		GasCap:          config.DefaultGasCap,
		ProposerAddress: suite.Ctx.BlockHeader().ProposerAddress,
	})
	require.NoError(t, err)

	var erc20DeployTx *types.MsgEthereumTx
	if enableFeemarket {
		erc20DeployTx = types.NewTxContract(
			chainID,
			nonce,
			nil,     // amount
			res.Gas, // gasLimit
			nil,     // gasPrice
			suite.App.FeeMarketKeeper.GetBaseFee(suite.Ctx),
			big.NewInt(1),
			data,                   // input
			&ethtypes.AccessList{}, // accesses
		)
	} else {
		erc20DeployTx = types.NewTxContract(
			chainID,
			nonce,
			nil,     // amount
			res.Gas, // gasLimit
			nil,     // gasPrice
			nil, nil,
			data, // input
			nil,  // accesses
		)
	}

	erc20DeployTx.From = suite.Address.Bytes()
	err = erc20DeployTx.Sign(ethtypes.LatestSignerForChainID(chainID), signer)
	require.NoError(t, err)
	rsp, err := suite.App.EvmKeeper.EthereumTx(ctx, erc20DeployTx)
	require.NoError(t, err)
	require.Empty(t, rsp.VmError)
	return crypto.CreateAddress(suite.Address, nonce)
}

// DeployTestContract deploy a test erc20 contract and returns the contract address
func (suite *EVMTestSuiteWithAccount) DeployTestContract(
	owner common.Address,
	supply *big.Int,
	enableFeemarket bool,
	queryClient types.QueryClient,
	signer keyring.Signer,
) common.Address {
	return suite.DeployTestContractWithT(owner, supply, enableFeemarket, queryClient, signer, suite.T())
}
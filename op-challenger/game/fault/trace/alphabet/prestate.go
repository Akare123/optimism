package alphabet

import (
	"context"

	"github.com/ethereum-optimism/optimism/cannon/mipsevm"
	"github.com/ethereum-optimism/optimism/op-challenger/game/fault/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var absolutePrestate = common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000060")

var _ types.PrestateProvider = (*AlphabetPrestateProvider)(nil)

type AlphabetPrestateProvider struct{}

func NewPrestateProvider() *AlphabetPrestateProvider {
	return &AlphabetPrestateProvider{}
}

func (ap *AlphabetPrestateProvider) AbsolutePreStateCommitment(_ context.Context) (common.Hash, error) {
	hash := common.BytesToHash(crypto.Keccak256(absolutePrestate))
	hash[0] = mipsevm.VMStatusUnfinished
	return hash, nil
}

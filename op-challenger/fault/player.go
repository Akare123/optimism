package fault

import (
	"context"
	"fmt"

	"github.com/ethereum-optimism/optimism/op-challenger/config"
	"github.com/ethereum-optimism/optimism/op-challenger/fault/alphabet"
	"github.com/ethereum-optimism/optimism/op-challenger/fault/cannon"
	"github.com/ethereum-optimism/optimism/op-challenger/fault/contracts"
	"github.com/ethereum-optimism/optimism/op-challenger/fault/types"
	"github.com/ethereum-optimism/optimism/op-service/txmgr"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
)

type Actor interface {
	Act(ctx context.Context) error
}

type GameInfo interface {
	GetGameStatus(context.Context) (types.GameStatus, error)
	GetClaimCount(context.Context) (uint64, error)
}

type GamePlayer struct {
	agent                   Actor
	agreeWithProposedOutput bool
	caller                  GameInfo
	logger                  log.Logger
}

func NewGamePlayer(
	ctx context.Context,
	logger log.Logger,
	cfg *config.Config,
	addr common.Address,
	txMgr txmgr.TxManager,
	client *ethclient.Client,
) (*GamePlayer, error) {
	logger = logger.New("game", addr)

	loader, err := contracts.NewFaultDisputeGameAtAddress(addr, client)
	if err != nil {
		return nil, fmt.Errorf("failed to bind the fault dispute game contract: %w", err)
	}

	gameDepth, err := loader.FetchGameDepth(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch the game depth: %w", err)
	}

	var provider types.TraceProvider
	var updater types.OracleUpdater
	switch cfg.TraceType {
	case config.TraceTypeCannon:
		provider, err = cannon.NewTraceProvider(ctx, logger, cfg, client, addr)
		if err != nil {
			return nil, fmt.Errorf("create cannon trace provider: %w", err)
		}
		updater, err = cannon.NewOracleUpdater(ctx, logger, txMgr, addr, client)
		if err != nil {
			return nil, fmt.Errorf("failed to create the cannon updater: %w", err)
		}
	case config.TraceTypeAlphabet:
		provider = alphabet.NewTraceProvider(cfg.AlphabetTrace, gameDepth)
		updater = alphabet.NewOracleUpdater(logger)
	default:
		return nil, fmt.Errorf("unsupported trace type: %v", cfg.TraceType)
	}

	if err := ValidateAbsolutePrestate(ctx, provider, loader); err != nil {
		return nil, fmt.Errorf("failed to validate absolute prestate: %w", err)
	}

	responder, err := NewFaultResponder(logger, txMgr, addr)
	if err != nil {
		return nil, fmt.Errorf("failed to create the responder: %w", err)
	}

	caller, err := contracts.NewFaultDisputeGameAtAddress(addr, client)
	if err != nil {
		return nil, fmt.Errorf("failed to bind the fault contract: %w", err)
	}

	return &GamePlayer{
		agent:                   NewAgent(loader, int(gameDepth), provider, responder, updater, cfg.AgreeWithProposedOutput, logger),
		agreeWithProposedOutput: cfg.AgreeWithProposedOutput,
		caller:                  caller,
		logger:                  logger,
	}, nil
}

func (g *GamePlayer) ProgressGame(ctx context.Context) bool {
	g.logger.Trace("Checking if actions are required")
	if err := g.agent.Act(ctx); err != nil {
		g.logger.Error("Error when acting on game", "err", err)
	}
	if status, err := g.caller.GetGameStatus(ctx); err != nil {
		g.logger.Warn("Unable to retrieve game status", "err", err)
	} else {
		g.logGameStatus(ctx, status)
		return status != types.GameStatusInProgress
	}
	return false
}

func (g *GamePlayer) logGameStatus(ctx context.Context, status types.GameStatus) {
	if status == types.GameStatusInProgress {
		claimCount, err := g.caller.GetClaimCount(ctx)
		if err != nil {
			g.logger.Error("Failed to get claim count for in progress game", "err", err)
			return
		}
		g.logger.Info("Game info", "claims", claimCount, "status", status)
		return
	}
	var expectedStatus types.GameStatus
	if g.agreeWithProposedOutput {
		expectedStatus = types.GameStatusChallengerWon
	} else {
		expectedStatus = types.GameStatusDefenderWon
	}
	if expectedStatus == status {
		g.logger.Info("Game won", "status", status)
	} else {
		g.logger.Error("Game lost", "status", status)
	}
}
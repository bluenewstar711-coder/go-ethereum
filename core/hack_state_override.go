// HACK_STATE_OVERRIDE — shadowfork support.
//
// This file is NOT upstream go-ethereum. It lets a shadowfork rewrite storage
// slots and account balances it does not hold the keys for (contract owners,
// proposers, sequencers inherited from the forked chain), so the fork can
// control them.
//
// Ported to v1.17.7 from v1.17.6-hack1, where it lived in
// consensus/beacon/consensus.go inside Beacon.Finalize. Upstream commit
// bbedffe56 ("tests: update test fixture to v21", #35835) moved withdrawals
// processing out of Finalize — which is now a near-empty shell — into
// core.PostExecution, so the hook moved with it.
//
// The hook point is load-bearing: the mutation must land in the state root
// computed by BOTH block building and block verification, or the two
// disagree and the chain stalls. PostExecution satisfies this — it is called
// by core/state_processor.go (verification) and core/chain_makers.go
// (building) alike, the same property Finalize had.

package core

import (
	"fmt"
	"math/big"
	"os"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types/bal"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"
)

// hackStateOverride applies all configured state overrides at the given block number.
//
// Two configuration styles are supported:
//
// Legacy (single override, backwards-compatible):
//
//	HACK_STATE_OVERRIDE_BLOCK=<blockNum>
//	HACK_STATE_OVERRIDE_ADDRESS=<hex address>
//	HACK_STATE_OVERRIDE_SLOT=<hex slot>
//	HACK_STATE_OVERRIDE_VALUE=<hex value>
//	HACK_STATE_OVERRIDE_BALANCE=<wei, decimal or 0x hex>
//
// Indexed (multiple overrides, N = 0, 1, 2, ...):
//
//	HACK_STATE_OVERRIDE_0_BLOCK=<blockNum>
//	HACK_STATE_OVERRIDE_0_ADDRESS=<hex address>
//	HACK_STATE_OVERRIDE_0_SLOT=<hex slot>
//	HACK_STATE_OVERRIDE_0_VALUE=<hex value>
//	HACK_STATE_OVERRIDE_1_BLOCK=<blockNum>
//	...
//
// Zero block number is treated as disabled. Indexed scanning stops at the
// first gap (an index whose _BLOCK var is unset) so indices must be consecutive.
//
// When Amsterdam (EIP-7928) is active, every mutation must also be recorded in
// the block-level access list, otherwise the BAL committed in the header will
// not match what a verifying node reconstructs and the block gets rejected.
func hackStateOverride(state vm.StateDB, blockNumber uint64, isAmsterdam bool, blockAccessIndex uint32, accessList *bal.ConstructionBlockAccessList) {
	// Legacy single-override (backwards-compatible).
	applyHackOverride(state, blockNumber,
		os.Getenv("HACK_STATE_OVERRIDE_BLOCK"),
		os.Getenv("HACK_STATE_OVERRIDE_ADDRESS"),
		os.Getenv("HACK_STATE_OVERRIDE_SLOT"),
		os.Getenv("HACK_STATE_OVERRIDE_VALUE"),
		os.Getenv("HACK_STATE_OVERRIDE_BALANCE"),
		isAmsterdam, blockAccessIndex, accessList,
	)
	// Indexed multi-override: scan until the first gap.
	for i := 0; ; i++ {
		prefix := fmt.Sprintf("HACK_STATE_OVERRIDE_%d_", i)
		blockStr := os.Getenv(prefix + "BLOCK")
		if blockStr == "" {
			break
		}
		applyHackOverride(state, blockNumber,
			blockStr,
			os.Getenv(prefix+"ADDRESS"),
			os.Getenv(prefix+"SLOT"),
			os.Getenv(prefix+"VALUE"),
			os.Getenv(prefix+"BALANCE"),
			isAmsterdam, blockAccessIndex, accessList,
		)
	}
}

// applyHackOverride writes a storage slot and/or sets a balance if blockNumber matches.
// SLOT+VALUE set a storage slot; BALANCE sets the account balance (in wei, decimal or 0x hex).
func applyHackOverride(state vm.StateDB, blockNumber uint64, overrideBlockStr, addr, slot, value, balance string, isAmsterdam bool, blockAccessIndex uint32, accessList *bal.ConstructionBlockAccessList) {
	overrideBlock, _ := strconv.ParseUint(overrideBlockStr, 10, 64)
	if overrideBlock == 0 || blockNumber != overrideBlock {
		return
	}
	target := common.HexToAddress(addr)
	if slot != "" && value != "" {
		fmt.Printf("===== HACK_STATE_OVERRIDE firing at block %d addr=%s slot=%s value=%s\n",
			blockNumber, addr, slot, value)
		slotKey, slotValue := common.HexToHash(slot), common.HexToHash(value)
		state.SetState(target, slotKey, slotValue)
		if isAmsterdam {
			accessList.StorageWrite(blockAccessIndex, target, slotKey, slotValue)
		}
	}
	if balance != "" {
		targetBal, ok := new(big.Int).SetString(balance, 0) // supports "0x..." hex and decimal
		if ok {
			targetU256 := uint256.MustFromBig(targetBal)
			currentBal := state.GetBalance(target)
			if currentBal.Cmp(targetU256) < 0 {
				diff := new(uint256.Int).Sub(targetU256, currentBal)
				fmt.Printf("===== HACK_BALANCE_OVERRIDE firing at block %d addr=%s adding %s wei (current=%s target=%s)\n",
					blockNumber, addr, diff.String(), currentBal.String(), targetU256.String())
				state.AddBalance(target, diff, tracing.BalanceChangeUnspecified)
				if isAmsterdam {
					accessList.BalanceChange(blockAccessIndex, target, targetU256)
				}
			}
		}
	}
}
